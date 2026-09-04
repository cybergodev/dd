package internal

import (
	"sync"
	"sync/atomic"
	"time"
)

// RateLimitStrategy defines the strategy for rate limiting log messages.
type RateLimitStrategy int

const (
	// RateLimitStrategyDrop drops messages when rate limit is exceeded.
	RateLimitStrategyDrop RateLimitStrategy = iota
	// RateLimitStrategySample samples messages when rate limit is exceeded (1 in N).
	RateLimitStrategySample
	// RateLimitStrategyThrottle throttles messages to the configured rate.
	// NOTE: in this non-blocking implementation it behaves like
	// RateLimitStrategyDrop — over-limit messages are dropped rather than
	// delayed. Retained for API completeness and future non-blocking throttle
	// strategies; do not rely on it for true backpressure today.
	RateLimitStrategyThrottle
)

// RateLimitConfig configures the rate limiter for preventing log flooding.
// Rate limiting protects against DoS attacks via excessive logging and helps
// maintain system stability under load.
type RateLimitConfig struct {
	// MaxMessagesPerSecond is the maximum number of messages allowed per second.
	// Set to 0 to disable message rate limiting.
	// Default: 10000 messages/second
	MaxMessagesPerSecond int

	// MaxBytesPerSecond is the maximum bytes allowed per second.
	// Set to 0 to disable byte rate limiting.
	// Default: 10MB/second
	MaxBytesPerSecond int64

	// BurstSize allows temporary bursts above the rate limit.
	// This is useful for handling sudden spikes in log volume.
	// Default: 1000 messages
	BurstSize int

	// Strategy determines how to handle rate-limited messages.
	// Default: RateLimitStrategyDrop
	Strategy RateLimitStrategy

	// SamplingRate is used when Strategy is RateLimitStrategySample.
	// It determines 1 in N messages to keep when rate limited.
	// Default: 100 (keep 1 in 100 messages)
	SamplingRate int
}

// DefaultRateLimitConfig returns a RateLimitConfig with sensible defaults.
// Defaults: 10000 messages/sec, 10MB/sec, burst of 1000, drop strategy.
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		MaxMessagesPerSecond: 10000,
		MaxBytesPerSecond:    10 * 1024 * 1024, // 10MB
		BurstSize:            1000,
		Strategy:             RateLimitStrategyDrop,
		SamplingRate:         100,
	}
}

// RateLimiter implements per-second rate limiting for log messages: fixed
// per-second budgets (messages, bytes) refilled once per wall-clock second,
// plus a burst allowance on top of the message budget.
// It uses a combination of atomic operations and mutex for thread-safe access.
// The mutex is only used for second boundary transitions to avoid TOCTOU races.
type RateLimiter struct {
	config *RateLimitConfig

	// Mutex for second boundary transitions
	secondMu sync.Mutex

	// Token bucket state (atomic)
	tokens           atomic.Int64 // Current number of tokens
	byteTokens       atomic.Int64 // Current byte tokens
	messageCount     atomic.Int64 // Messages in current second
	byteCount        atomic.Int64 // Bytes in current second
	currentSecond    atomic.Int64 // Current second (Unix timestamp)
	rateLimitedCount atomic.Int64 // Total rate-limited messages

	// Sampling state
	sampleCounter atomic.Int64
}

// NewRateLimiter creates a new RateLimiter with the given configuration.
// If config is nil, defaults are used.
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	rl := &RateLimiter{
		config: config,
	}

	// Initialize token buckets
	rl.tokens.Store(int64(config.BurstSize))
	rl.byteTokens.Store(config.MaxBytesPerSecond)
	rl.currentSecond.Store(time.Now().Unix())

	return rl
}

// maybeResetSecond advances the per-second accounting window when the wall-clock
// second changes. It uses a mutex with double-checked locking so that
// second-boundary resets are safe under concurrency (preventing TOCTOU races on
// the per-second counters and token buckets). Within a second it is a lock-free
// atomic load, keeping the hot path cheap.
func (rl *RateLimiter) maybeResetSecond(now time.Time) {
	nowSec := now.Unix()
	if nowSec == rl.currentSecond.Load() {
		return
	}
	rl.secondMu.Lock()
	defer rl.secondMu.Unlock()
	// Double-check after acquiring the lock: another goroutine may have
	// already advanced the window while we were waiting.
	if nowSec == rl.currentSecond.Load() {
		return
	}
	// SECURITY: only advance the window FORWARD. A backwards wall-clock step
	// (NTP correction, VM snapshot resume, manual change) used to trigger
	// this reset too, fully refilling the budgets on every observed second
	// transition — refreshing the flood-protection budget many times per
	// actual second. Treat a backwards step as "still the same window".
	if nowSec < rl.currentSecond.Load() {
		return
	}
	rl.currentSecond.Store(nowSec)
	rl.messageCount.Store(0)
	rl.byteCount.Store(0)
	if rl.config.MaxMessagesPerSecond > 0 {
		rl.tokens.Store(int64(rl.config.BurstSize))
	}
	if rl.config.MaxBytesPerSecond > 0 {
		rl.byteTokens.Store(rl.config.MaxBytesPerSecond)
	}
}

// AllowMessage applies only the per-message rate limit (message count plus the
// burst token bucket). It is the cheap pre-format gate on the logger hot path,
// where the formatted message size is not yet known.
//
// Returns true if the message is allowed to proceed, false if it should be
// dropped or sampled per the configured Strategy.
func (rl *RateLimiter) AllowMessage() bool {
	if rl == nil || rl.config == nil || rl.config.MaxMessagesPerSecond <= 0 {
		return true
	}
	rl.maybeResetSecond(time.Now())
	if rl.messageCount.Add(1) <= int64(rl.config.MaxMessagesPerSecond) {
		return true
	}
	// Over the per-second budget: try to spend a burst token.
	if rl.tokens.Add(-1) < 0 {
		rl.tokens.Add(1) // restore: no burst budget left
		return !rl.handleRateLimited()
	}
	return true
}

// AllowBytes applies only the byte rate limit (byte count plus the byte token
// bucket) for a message of the given size. It is the post-format gate, where
// the formatted message length is known.
//
// Returns true if the message is allowed to proceed, false if it should be
// dropped or sampled per the configured Strategy.
func (rl *RateLimiter) AllowBytes(msgSize int) bool {
	if rl == nil || rl.config == nil || rl.config.MaxBytesPerSecond <= 0 || msgSize <= 0 {
		return true
	}
	rl.maybeResetSecond(time.Now())
	if rl.byteCount.Add(int64(msgSize)) <= rl.config.MaxBytesPerSecond {
		return true
	}
	// Over the per-second byte budget: try to spend byte tokens.
	if rl.byteTokens.Add(-int64(msgSize)) < 0 {
		rl.byteTokens.Add(int64(msgSize)) // restore tokens
		rl.byteCount.Add(-int64(msgSize)) // don't count rejected bytes
		// The message already passed AllowMessage, which incremented
		// messageCount; since it is now rejected by the byte gate, roll back
		// that count so the per-message budget isn't consumed by dropped
		// messages. AllowMessage is a no-op (no increment) when
		// MaxMessagesPerSecond <= 0, so only roll back when it is active.
		if rl.config.MaxMessagesPerSecond > 0 {
			decrementFloored(&rl.messageCount)
		}
		return !rl.handleRateLimited()
	}
	return true
}

// decrementFloored decrements v by one, never below zero.
//
// The byte gate's messageCount rollback assumed AllowMessage had incremented
// the same limiter instance in the same accounting window, but neither holds
// under concurrency: the logger loads its rate limiter twice (AllowMessage in
// shouldLog, AllowBytes in logCoreWithDepth), and SetSecurityConfig can swap
// the instance between the two — or maybeResetSecond can zero the counter
// between the increment and the rollback. A plain Add(-1) then drives
// messageCount negative, and a negative count lets the window admit more than
// MaxMessagesPerSecond messages until the next reset. Flooring the rollback at
// zero keeps the invariant "messageCount >= 0"; when the increment is missing
// the rollback is skipped, which can only over-consume a message slot
// (fail-closed). Off the happy path: this runs only when the byte gate
// rejects a message.
func decrementFloored(v *atomic.Int64) {
	for {
		current := v.Load()
		if current <= 0 {
			return
		}
		if v.CompareAndSwap(current, current-1) {
			return
		}
	}
}

// ShouldRateLimit checks whether a message of the given size should be rate
// limited, applying both the message-count and byte limits in a single call.
// Returns true if the message should be dropped/throttled, false if it should
// be processed.
//
// This convenience is for callers that already know the message size. The
// logger hot path instead calls AllowMessage (pre-format, cheap) and AllowBytes
// (post-format, once the size is known) separately, so that byte limiting
// actually takes effect — calling ShouldRateLimit(0) leaves the byte limit inert.
func (rl *RateLimiter) ShouldRateLimit(msgSize int) bool {
	if rl == nil || rl.config == nil {
		return false
	}
	if rl.config.MaxMessagesPerSecond <= 0 && rl.config.MaxBytesPerSecond <= 0 {
		return false
	}
	// Message gate first; if it drops the message, short-circuit before the
	// byte gate (matching the original implementation's ordering).
	return !rl.AllowMessage() || !rl.AllowBytes(msgSize)
}

// handleRateLimited handles the rate limit strategy.
func (rl *RateLimiter) handleRateLimited() bool {
	rl.rateLimitedCount.Add(1)

	switch rl.config.Strategy {
	case RateLimitStrategySample:
		// Keep 1 in N messages
		samplingRate := rl.config.SamplingRate
		if samplingRate <= 0 {
			return true // Invalid sampling rate, drop to be safe
		}
		counter := rl.sampleCounter.Add(1)
		return counter%int64(samplingRate) != 0

	case RateLimitStrategyThrottle:
		// For throttle, we'd need blocking behavior which isn't suitable
		// for the hot path. Fall through to drop behavior.
		fallthrough

	default:
		// RateLimitStrategyDrop and any unknown strategy: drop the message
		return true
	}
}

// GetStats returns current rate limiter statistics.
type RateLimitStats struct {
	Tokens            int64 // Current message tokens
	ByteTokens        int64 // Current byte tokens
	MessageCount      int64 // Messages in current second
	ByteCount         int64 // Bytes in current second
	RateLimitedCount  int64 // Total rate-limited messages
	CurrentSecond     int64 // Current second (Unix timestamp)
	MaxMessagesPerSec int   // Configured max messages/sec
	MaxBytesPerSec    int64 // Configured max bytes/sec
}

// Stats returns current rate limiter statistics for monitoring.
func (rl *RateLimiter) Stats() RateLimitStats {
	if rl == nil {
		return RateLimitStats{}
	}

	return RateLimitStats{
		Tokens:            rl.tokens.Load(),
		ByteTokens:        rl.byteTokens.Load(),
		MessageCount:      rl.messageCount.Load(),
		ByteCount:         rl.byteCount.Load(),
		RateLimitedCount:  rl.rateLimitedCount.Load(),
		CurrentSecond:     rl.currentSecond.Load(),
		MaxMessagesPerSec: rl.config.MaxMessagesPerSecond,
		MaxBytesPerSec:    rl.config.MaxBytesPerSecond,
	}
}

// Reset resets the rate limiter state.
func (rl *RateLimiter) Reset() {
	if rl == nil {
		return
	}

	rl.tokens.Store(int64(rl.config.BurstSize))
	rl.byteTokens.Store(rl.config.MaxBytesPerSecond)
	rl.messageCount.Store(0)
	rl.byteCount.Store(0)
	rl.rateLimitedCount.Store(0)
	rl.sampleCounter.Store(0)
	rl.currentSecond.Store(time.Now().Unix())
}

// Clone creates a copy of the RateLimitConfig.
func (c *RateLimitConfig) Clone() *RateLimitConfig {
	if c == nil {
		return nil
	}

	return &RateLimitConfig{
		MaxMessagesPerSecond: c.MaxMessagesPerSecond,
		MaxBytesPerSecond:    c.MaxBytesPerSecond,
		BurstSize:            c.BurstSize,
		Strategy:             c.Strategy,
		SamplingRate:         c.SamplingRate,
	}
}
