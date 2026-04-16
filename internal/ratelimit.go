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

// RateLimiter implements a token bucket rate limiter for log messages.
// It uses a combination of atomic operations and mutex for thread-safe access.
// The mutex is only used for second boundary transitions to avoid TOCTOU races.
type RateLimiter struct {
	config *RateLimitConfig

	// Mutex for second boundary transitions
	secondMu sync.Mutex

	// Token bucket state (atomic)
	tokens           atomic.Int64 // Current number of tokens
	byteTokens       atomic.Int64 // Current byte tokens
	lastRefill       atomic.Int64 // Last refill time (Unix nanoseconds)
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
	rl.lastRefill.Store(time.Now().UnixNano())
	rl.currentSecond.Store(time.Now().Unix())

	return rl
}

// ShouldRateLimit checks if a message of the given size should be rate limited.
// Returns true if the message should be dropped/throttled, false if it should be processed.
// This method uses a mutex for second boundary transitions to prevent TOCTOU races,
// but uses atomic operations for the hot path within each second.
func (rl *RateLimiter) ShouldRateLimit(msgSize int) bool {
	if rl == nil || rl.config == nil {
		return false
	}

	// Quick check: if both limits are disabled, never rate limit
	if rl.config.MaxMessagesPerSecond <= 0 && rl.config.MaxBytesPerSecond <= 0 {
		return false
	}

	now := time.Now()
	nowNano := now.UnixNano()
	nowSec := now.Unix()

	// Check if we've moved to a new second (with mutex to prevent TOCTOU race)
	currentSec := rl.currentSecond.Load()
	if nowSec != currentSec {
		rl.secondMu.Lock()
		// Double-check after acquiring lock (another goroutine may have updated)
		currentSec = rl.currentSecond.Load()
		if nowSec != currentSec {
			// Reset counters for new second
			rl.currentSecond.Store(nowSec)
			rl.messageCount.Store(0)
			rl.byteCount.Store(0)

			// Refill token buckets
			if rl.config.MaxMessagesPerSecond > 0 {
				rl.tokens.Store(int64(rl.config.BurstSize))
			}
			if rl.config.MaxBytesPerSecond > 0 {
				rl.byteTokens.Store(rl.config.MaxBytesPerSecond)
			}
		}
		rl.secondMu.Unlock()
	}

	// Check message rate limit
	if rl.config.MaxMessagesPerSecond > 0 {
		msgCount := rl.messageCount.Add(1)
		if msgCount > int64(rl.config.MaxMessagesPerSecond) {
			// Check burst capacity
			tokens := rl.tokens.Add(-1)
			if tokens < 0 {
				rl.tokens.Add(1) // Restore token
				return rl.handleRateLimited()
			}
		}
	}

	// Check byte rate limit
	if rl.config.MaxBytesPerSecond > 0 {
		byteCount := rl.byteCount.Add(int64(msgSize))
		if byteCount > rl.config.MaxBytesPerSecond {
			// Check byte token bucket
			byteTokens := rl.byteTokens.Add(-int64(msgSize))
			if byteTokens < 0 {
				rl.byteTokens.Add(int64(msgSize)) // Restore tokens
				return rl.handleRateLimited()
			}
		}
	}

	// Update last refill time
	rl.lastRefill.Store(nowNano)

	return false
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
	rl.lastRefill.Store(time.Now().UnixNano())
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
