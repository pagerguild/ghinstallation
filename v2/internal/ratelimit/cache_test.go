package ratelimit

import (
	"context"
	"sync"
	"testing"
)

// Test rate limiter struct from the real NewRateLimiter function
func TestRateLimiterCache_Basic(t *testing.T) {
	cache := NewRateLimiterCache(10, 2) // 2 max tenants

	// Get a new rate limiter
	rl1 := cache.GetRateLimiter(1)

	if rl1 == nil {
		t.Fatal("expected non-nil rate limiter")
	}

	// Get the same rate limiter again
	rl2 := cache.GetRateLimiter(1)
	if rl1 != rl2 {
		t.Fatal("expected the same rate limiter instance for the same installationID")
	}

	// Get a different rate limiter for a different installation ID
	rl3 := cache.GetRateLimiter(2)
	if rl1 == rl3 {
		t.Fatal("expected different rate limiter instances for different installationIDs")
	}

	if rl1.Acquire(context.Background()) != nil {
		t.Fatal("API should not give errors here")
	}
	if rl2.Acquire(context.Background()) != nil {
		t.Fatal("API should not give errors here")
	}
	if rl3.Acquire(context.Background()) != nil {
		t.Fatal("API should not give errors here")
	}
}

// Test cache eviction and proper closing behavior
func TestRateLimiterCache_Eviction(t *testing.T) {
	cache := NewRateLimiterCache(10, 2) // 2 max tenants in the LRU cache

	// Add two rate limiters
	_ = cache.GetRateLimiter(1)
	_ = cache.GetRateLimiter(2)

	// Insert a third rate limiter, evicting the first one
	_ = cache.GetRateLimiter(3)

	// At this point, rl1 should be evicted
	_, exists := cache.rateLimiters.Get(1)
	if exists {
		t.Fatal("expected rate limiter for installation 1 to be evicted")
	}

	exists = cache.rateLimiters.Contains(int64(2))
	if !exists {
		t.Fatal("expected rate limiter for installation 2 to still be in cache")
	}

	_, exists = cache.rateLimiters.Get(int64(3))
	if !exists {
		t.Fatal("expected rate limiter for installation 3 to still be in cache")
	}
}

// Test cache concurrency with multiple goroutines
func TestRateLimiterCache_Concurrency(t *testing.T) {
	cache := NewRateLimiterCache(10, 2) // 2 max tenants

	var wg sync.WaitGroup
	const numGoroutines = 100
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(installationID int64) {
			defer wg.Done()
			cache.GetRateLimiter(installationID)
		}(int64(i % 2)) // Two installation IDs to force cache contention
	}

	wg.Wait()

	// Verify only two rate limiters in the cache, as the cache can hold only 2 tenants
	if cache.rateLimiters.Len() != 2 {
		t.Fatalf("expected 2 rate limiters in the cache, got %d", cache.rateLimiters.Len())
	}
}
