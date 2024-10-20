package ratelimit

import (
	"context"
	"net/http"
)

// RateLimiter combines concurrency control with GitHub header-based rate limiting.
type RateLimiter struct {
	semaphore     semaphore
	headerLimiter *githubHeaderRateLimiter
}

// NewRateLimiter creates a new rate limiter with both concurrency control and GitHub header-based rate limiting.
func NewRateLimiter(maxConcurrent int64) *RateLimiter {
	return &RateLimiter{
		semaphore:     NewSemaphore(int(maxConcurrent)),
		headerLimiter: newGitHubHeaderRateLimiter(maxConcurrent),
	}
}

// Acquire blocks based on both throttling and concurrency limits.
func (r *RateLimiter) Acquire(ctx context.Context) error {
	if err := r.headerLimiter.Acquire(ctx); err != nil {
		return err
	}

	return r.semaphore.Acquire(ctx)
}

// Release a slot and process the GitHub API response for potential throttling.
func (r *RateLimiter) Release(resp *http.Response) {
	r.semaphore.Release()
	r.headerLimiter.AddResponse(resp)
}

// Close releases all resources.
func (r *RateLimiter) Close() {
	r.semaphore.Close()
	r.headerLimiter.Close()
}
