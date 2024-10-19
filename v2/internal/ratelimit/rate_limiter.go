package ratelimit

import (
	"context"
	"net/http"
	"time"
)

/*
 */

const (
	thresholdRequestsLeftForThrottle = 100
)

// A rate limit manager for rate limiting GitHub API requests.
// Requests pass through unhindered until a response indicates that there are
// fewer than [resetThreshold] remaining before GitHub will start returning
// HTTP errors.
//
// At that time, [Wait()] will block until the specified amount of time has
// passed.
type RateLimiter struct {
	resetThreshold int64               // Throttle when remaining requests are below this threshold
	responses      chan *http.Response // Channel to process HTTP responses
	throttle       chan struct{}       // Channel that blocks when throttling
	semaphore      Semaphore
}

// newRateLimiter creates a new rate limit throttle using response channels.
func newRateLimiter(maxConcurrent int64) *RateLimiter {
	throttle := &RateLimiter{
		resetThreshold: maxConcurrent,
		responses:      make(chan *http.Response, maxConcurrent), // If/when we throttle, any in-flight requests should not be blocked writing to this channel.  Hence the specific size.
		throttle:       make(chan struct{}),                      // This channel blocks when throttling
		semaphore:      make(Semaphore, maxConcurrent),
	}
	close(throttle.throttle)

	// Start the throttling goroutine
	go throttle.manageThrottle()

	return throttle
}

func (r RateLimiter) close() {
	close(r.responses)
	close(r.throttle)
	close(r.semaphore)
}

// manageThrottle manages rate limits based on incoming responses.
func (r *RateLimiter) manageThrottle() {
	for {
		select {
		case resp, ok := <-r.responses:
			if !ok {
				return // Channel closed, exit goroutine
			}
			if resp == nil {
				continue
			}

			// Parse the rate limit headers
			rateLimitInfo := NewGitHubRateLimitInfo(resp)

			// If remaining requests drop below the threshold, start throttling
			if rateLimitInfo.Remaining <= r.resetThreshold {
				// Close the throttleActive channel to block requests
				r.startThrottling(rateLimitInfo.TimeToReset())
			}
		}
	}
}

func (r *RateLimiter) startThrottling(resetDuration time.Duration) {
	// Open the throttleActive channel to block requests
	r.throttle = make(chan struct{})

	// Set up a timer to reopen the throttle after the reset time
	go func() {
		timer := time.NewTimer(resetDuration)
		defer timer.Stop() // Ensure the timer is cleaned up

		<-timer.C
		close(r.throttle)
	}()
}

// Acquire blocks requests when throttling is active.
func (r *RateLimiter) Acquire(ctx context.Context) error {
	select {
	case <-r.throttle:
		// Pass through.
	case <-ctx.Done():
		// Context expired or was cancelled
		return ctx.Err()
	}
	return r.semaphore.Acquire(ctx)
}

// Add a response to the throttle queue for processing.
func (r *RateLimiter) Release(resp *http.Response) {
	r.semaphore.Release()
	r.responses <- resp
}
