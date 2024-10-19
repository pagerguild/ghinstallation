package ratelimit

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
)

// The rateLimiterCache is the primary entrypoing into rate limiters for the
// calling application.  It holds a LRU cache of rate limiter objects.  Each
// time a GitHub Installation transport is created, we check to see if there is
// already a rate limiter for that installation ID.  In this way we can share
// rate limit checking across all clients for that installation ID.
//
// The rate limiter enforces both primary (requests per hour) and secondary
// (concurrent requests) rate limits.
type rateLimiterCache struct {
	rateLimiters *lru.Cache // LRU cache for managing semaphores
	limit        int64      // concurrency limit per semaphore
	mu           sync.RWMutex
}

func NewRateLimiterCache(maxConcurrent int, maxTenants int) *rateLimiterCache {
	lruCache, _ := lru.NewWithEvict(maxTenants, func(_, value interface{}) {
		(value.(RateLimiter)).close()
	})

	return &rateLimiterCache{
		rateLimiters: lruCache,
		limit:        int64(maxConcurrent),
	}
}

func (sm *rateLimiterCache) GetRateLimiter(installationID int64) *RateLimiter {
	// Check if semaphore exists in the LRU cache
	sm.mu.RLock()
	sem, exists := sm.rateLimiters.Get(installationID)
	sm.mu.RUnlock()

	if exists {
		return sem.(*RateLimiter)
	}

	// Lock for writing if semaphore does not exist
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check if the rate limiter was added by another goroutine
	if sem, exists := sm.rateLimiters.Get(installationID); exists {
		return sem.(*RateLimiter)
	}

	rl := newRateLimiter(sm.limit)
	sm.rateLimiters.Add(installationID, rl)
	return rl
}
