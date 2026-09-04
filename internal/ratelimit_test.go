package internal

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiter_BasicRateLimit(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 10,
		MaxBytesPerSecond:    0, // Disabled
		BurstSize:            5,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	// First messages should pass (up to MaxMessagesPerSecond + BurstSize)
	allowed := 0
	for i := 0; i < 20; i++ {
		if !rl.ShouldRateLimit(100) {
			allowed++
		}
	}

	// Should allow MaxMessagesPerSecond (10) + BurstSize (5) = 15
	if allowed != 15 {
		t.Errorf("Expected 15 messages allowed, got %d", allowed)
	}
}

// TestRateLimiter_LimitsDisabled consolidates the "limits off ⇒ everything
// passes" family: zero and negative message/byte limits both disable
// limiting regardless of burst settings.
func TestRateLimiter_LimitsDisabled(t *testing.T) {
	tests := []struct {
		name   string
		config *RateLimitConfig
		calls  int
	}{
		{"zero limits with burst", &RateLimitConfig{
			MaxMessagesPerSecond: 0, MaxBytesPerSecond: 0, BurstSize: 5,
			Strategy: RateLimitStrategyDrop,
		}, 100},
		{"all zero", &RateLimitConfig{
			MaxMessagesPerSecond: 0, MaxBytesPerSecond: 0, BurstSize: 0,
			Strategy: RateLimitStrategyDrop,
		}, 1000},
		{"negative values", &RateLimitConfig{
			MaxMessagesPerSecond: -1, MaxBytesPerSecond: -1, BurstSize: -1,
			Strategy: RateLimitStrategyDrop,
		}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.config)
			for i := 0; i < tt.calls; i++ {
				if rl.ShouldRateLimit(100) {
					t.Fatalf("message %d limited despite disabled limits", i)
				}
			}
		})
	}
}

// TestAllowBytesBoundaries covers the post-format byte gate directly: nil
// limiter, disabled limits, and zero/negative message sizes are no-ops.
func TestAllowBytesBoundaries(t *testing.T) {
	var nilLimiter *RateLimiter
	if !nilLimiter.AllowBytes(100) {
		t.Error("nil limiter should allow")
	}
	if !nilLimiter.AllowBytes(0) {
		t.Error("nil limiter should allow empty messages")
	}

	rl := NewRateLimiter(&RateLimitConfig{Strategy: RateLimitStrategyDrop})
	if !rl.AllowBytes(0) {
		t.Error("msgSize=0 should be allowed (nothing to account)")
	}
	if !rl.AllowBytes(-5) {
		t.Error("negative msgSize should be allowed")
	}
}

func TestRateLimiter_ByteRateLimit(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 0,   // Disabled
		MaxBytesPerSecond:    100, // 100 bytes/sec
		BurstSize:            10,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	// Small messages should pass
	allowed := 0
	for i := 0; i < 20; i++ {
		if !rl.ShouldRateLimit(5) { // 5 bytes each
			allowed++
		}
	}

	// Should allow about 100/5 = 20 messages (byte limit)
	// Plus burst buffer
	if allowed < 15 {
		t.Errorf("Expected at least 15 messages allowed, got %d", allowed)
	}
}

func TestRateLimiter_SampleStrategy(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 5,
		MaxBytesPerSecond:    0,
		BurstSize:            2,
		Strategy:             RateLimitStrategySample,
		SamplingRate:         2, // Keep 1 in 2
	}

	rl := NewRateLimiter(config)

	// After burst is exhausted, sampling should keep ~50%
	allowed := 0
	for i := 0; i < 100; i++ {
		if !rl.ShouldRateLimit(100) {
			allowed++
		}
	}

	// Should allow MaxMessagesPerSecond (5) + BurstSize (2) + ~50% of remaining
	// 5 + 2 = 7 guaranteed, then sampling from 93 messages = ~46 more
	if allowed < 30 || allowed > 70 {
		t.Errorf("Expected ~50 messages with sampling, got %d", allowed)
	}
}

func TestRateLimiter_Concurrency(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 100, // 100 messages/sec
		MaxBytesPerSecond:    0,
		BurstSize:            10, // +10 burst
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	var wg sync.WaitGroup
	var allowed atomic.Int64

	// Run 10 goroutines, each trying to send 100 messages (1000 total)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if !rl.ShouldRateLimit(100) {
					allowed.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// Should allow MaxMessagesPerSecond (100) + BurstSize (10) = 110
	// From 1000 total messages, expect at least 800 rate-limited
	stats := rl.Stats()
	if stats.RateLimitedCount < 800 {
		t.Errorf("Expected many rate-limited messages, got %d (allowed=%d)", stats.RateLimitedCount, allowed.Load())
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 10,
		MaxBytesPerSecond:    0,
		BurstSize:            5,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	// Exhaust the rate limit
	for i := 0; i < 20; i++ {
		rl.ShouldRateLimit(100)
	}

	stats := rl.Stats()
	if stats.RateLimitedCount == 0 {
		t.Error("Expected some rate-limited messages")
	}

	// Reset
	rl.Reset()

	stats = rl.Stats()
	if stats.RateLimitedCount != 0 {
		t.Errorf("Expected rate-limited count to be 0 after reset, got %d", stats.RateLimitedCount)
	}
	if stats.Tokens != int64(config.BurstSize) {
		t.Errorf("Expected tokens to be %d after reset, got %d", config.BurstSize, stats.Tokens)
	}
}

func TestRateLimiter_Stats(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 100,
		MaxBytesPerSecond:    1000,
		BurstSize:            10,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	stats := rl.Stats()
	if stats.MaxMessagesPerSec != 100 {
		t.Errorf("Expected MaxMessagesPerSec=100, got %d", stats.MaxMessagesPerSec)
	}
	if stats.MaxBytesPerSec != 1000 {
		t.Errorf("Expected MaxBytesPerSec=1000, got %d", stats.MaxBytesPerSec)
	}
	if stats.Tokens != 10 {
		t.Errorf("Expected initial tokens=10, got %d", stats.Tokens)
	}
}

func TestRateLimiter_NilSafety(t *testing.T) {
	var rl *RateLimiter

	// Should not panic
	if rl.ShouldRateLimit(100) {
		t.Error("Nil rate limiter should not rate limit")
	}

	stats := rl.Stats()
	if stats.MaxMessagesPerSec != 0 || stats.MaxBytesPerSec != 0 {
		t.Error("Nil rate limiter should return zero stats")
	}

	rl.Reset() // Should not panic
}

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	if config.MaxMessagesPerSecond != 10000 {
		t.Errorf("Expected default MaxMessagesPerSecond=10000, got %d", config.MaxMessagesPerSecond)
	}
	if config.MaxBytesPerSecond != 10*1024*1024 {
		t.Errorf("Expected default MaxBytesPerSecond=10MB, got %d", config.MaxBytesPerSecond)
	}
	if config.BurstSize != 1000 {
		t.Errorf("Expected default BurstSize=1000, got %d", config.BurstSize)
	}
	if config.Strategy != RateLimitStrategyDrop {
		t.Errorf("Expected default Strategy=Drop, got %d", config.Strategy)
	}
}

func TestRateLimitConfig_Clone(t *testing.T) {
	original := &RateLimitConfig{
		MaxMessagesPerSecond: 500,
		MaxBytesPerSecond:    1024,
		BurstSize:            50,
		Strategy:             RateLimitStrategySample,
		SamplingRate:         10,
	}

	cloned := original.Clone()

	if cloned == original {
		t.Error("Clone should return a new instance")
	}

	if cloned.MaxMessagesPerSecond != original.MaxMessagesPerSecond {
		t.Error("MaxMessagesPerSecond should be copied")
	}
	if cloned.MaxBytesPerSecond != original.MaxBytesPerSecond {
		t.Error("MaxBytesPerSecond should be copied")
	}
	if cloned.Strategy != original.Strategy {
		t.Error("Strategy should be copied")
	}

	// Modify original, ensure clone is unaffected
	original.MaxMessagesPerSecond = 999
	if cloned.MaxMessagesPerSecond == 999 {
		t.Error("Clone should not be affected by original modifications")
	}
}

func TestRateLimitConfig_CloneNil(t *testing.T) {
	var config *RateLimitConfig
	cloned := config.Clone()
	if cloned != nil {
		t.Error("Cloning nil should return nil")
	}
}

func TestNewRateLimiter_NilConfig(t *testing.T) {
	rl := NewRateLimiter(nil)

	if rl == nil {
		t.Fatal("NewRateLimiter should not return nil")
	}
	// The exact default values are pinned by TestDefaultRateLimitConfig;
	// here it is enough that a real config was installed.
	if rl.config == nil {
		t.Error("Nil config should be replaced with defaults, not stay nil")
	}
}

func TestRateLimiter_NewSecondReset(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 5,
		MaxBytesPerSecond:    0,
		BurstSize:            2,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	// Exhaust rate limit
	for i := 0; i < 20; i++ {
		rl.ShouldRateLimit(100)
	}

	// Simulate moving to next second
	rl.currentSecond.Store(time.Now().Unix() - 1)

	// Should reset and allow messages again
	if rl.ShouldRateLimit(100) {
		t.Error("Message should be allowed after second reset")
	}
}

// ============================================================================
// RATE LIMITER BOUNDARY TESTS
// ============================================================================

func TestRateLimiter_VeryLargeMessage(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 0,   // Disabled
		MaxBytesPerSecond:    100, // 100 bytes/sec
		BurstSize:            10,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	// Very large message should be rate limited immediately
	if !rl.ShouldRateLimit(10000) {
		t.Error("Very large message should be rate limited")
	}

	// Small messages should still pass until burst is exhausted
	allowed := 0
	for i := 0; i < 20; i++ {
		if !rl.ShouldRateLimit(5) {
			allowed++
		}
	}

	if allowed < 10 {
		t.Errorf("Expected at least 10 small messages allowed, got %d", allowed)
	}
}

func TestRateLimiter_VeryHighRate(t *testing.T) {
	// Very high rate should effectively allow all messages
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 1000000, // 1M messages/sec
		MaxBytesPerSecond:    0,
		BurstSize:            100000,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	allowed := 0
	for i := 0; i < 10000; i++ {
		if !rl.ShouldRateLimit(100) {
			allowed++
		}
	}

	// All should be allowed
	if allowed != 10000 {
		t.Errorf("Expected all 10000 messages allowed, got %d", allowed)
	}
}

func TestRateLimiter_BurstOnly(t *testing.T) {
	// Only burst, no sustained rate
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 1, // Very low
		MaxBytesPerSecond:    0,
		BurstSize:            50,
		Strategy:             RateLimitStrategyDrop,
	}

	rl := NewRateLimiter(config)

	// Should allow burst
	allowed := 0
	for i := 0; i < 100; i++ {
		if !rl.ShouldRateLimit(100) {
			allowed++
		}
	}

	// Should allow approximately MaxMessagesPerSecond + BurstSize
	if allowed < 40 || allowed > 60 {
		t.Errorf("Expected ~50 messages (burst), got %d", allowed)
	}
}

// TestRateLimiter_AllowBytesRollbackNeverNegative pins the byte-gate rollback
// floor. AllowBytes' messageCount rollback assumed a prior AllowMessage had
// incremented the same limiter in the same window, but the logger loads its
// rate limiter twice on the log path and SetSecurityConfig can swap the
// instance between the two gates (or maybeResetSecond can zero the counter in
// between). Standalone AllowBytes rejections model that shape: without the
// floor, each rejection drives messageCount further negative and the window
// admits more than MaxMessagesPerSecond messages until the next reset.
func TestRateLimiter_AllowBytesRollbackNeverNegative(t *testing.T) {
	config := &RateLimitConfig{
		MaxMessagesPerSecond: 5,
		MaxBytesPerSecond:    100, // tiny byte budget: every call below is rejected
		BurstSize:            0,
		Strategy:             RateLimitStrategyDrop,
	}
	rl := NewRateLimiter(config)

	// Byte-gate rejections without any matching AllowMessage increment.
	for i := 0; i < 100; i++ {
		if rl.AllowBytes(1000) {
			t.Fatal("oversized message should be rejected by the byte gate")
		}
	}

	if stats := rl.Stats(); stats.MessageCount < 0 {
		t.Fatalf("messageCount went negative (%d); the rollback must floor at zero", stats.MessageCount)
	}

	// The floored counter must still roll back a real AllowMessage increment
	// (same instance, same window): 5 fully-allowed messages, then a 6th that
	// passes the message gate but is rejected by the byte gate, leaves exactly
	// 5 counted — not 6 (no rollback) and not 4 (over-rollback).
	rl2 := NewRateLimiter(&RateLimitConfig{
		MaxMessagesPerSecond: 10,
		MaxBytesPerSecond:    100,
		BurstSize:            0,
		Strategy:             RateLimitStrategyDrop,
	})
	for i := 0; i < 5; i++ {
		if !rl2.AllowMessage() {
			t.Fatal("messages within budget must be allowed")
		}
		if !rl2.AllowBytes(10) {
			t.Fatal("small messages within the byte budget must be allowed")
		}
	}
	// The 6th message passes the message gate, then is rejected by the byte gate.
	if !rl2.AllowMessage() {
		t.Fatal("message within budget must be allowed")
	}
	if rl2.AllowBytes(1000) {
		t.Fatal("oversized message should be rejected by the byte gate")
	}
	if got := rl2.Stats().MessageCount; got != 5 {
		t.Fatalf("messageCount = %d after rollback, want 5", got)
	}
}
