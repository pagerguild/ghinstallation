package ratelimit

import (
	"context"
	"net/http"
	"time"
)

// githubHeaderRateLimiter manages rate limits based on GitHub rate limit headers.
type githubHeaderRateLimiter struct {
	resetThreshold int64               // Throttle when remaining requests are below this threshold
	responses      chan *http.Response // Channel to process HTTP responses
	throttle       chan struct{}       // Channel that blocks when throttling
}

func (r *githubHeaderRateLimiter) Acquire(ctx context.Context) error {
	select {
	case <-r.throttle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *githubHeaderRateLimiter) Close() {
	close(r.responses)
}

// newGitHubHeaderRateLimiter creates a new rate limiter based on GitHub headers.
func newGitHubHeaderRateLimiter(threshold int64) *githubHeaderRateLimiter {
	rateLimiter := &githubHeaderRateLimiter{
		resetThreshold: threshold,
		responses:      make(chan *http.Response, threshold),
		throttle:       make(chan struct{}),
	}
	close(rateLimiter.throttle)

	go rateLimiter.manageThrottle()
	return rateLimiter
}

// manageThrottle manages rate limits based on incoming responses.
func (r *githubHeaderRateLimiter) manageThrottle() {
	for {
		select {
		case resp, ok := <-r.responses:
			if !ok {
				return
			}
			if resp == nil {
				continue
			}

			rateLimitInfo := NewGitHubRateLimitInfo(resp)

			// If remaining requests drop below the threshold, start throttling
			if rateLimitInfo.Remaining <= r.resetThreshold {
				r.startThrottling(rateLimitInfo.TimeToReset())
			}
		}
	}
}

func (r *githubHeaderRateLimiter) startThrottling(resetDuration time.Duration) {
	r.throttle = make(chan struct{})
	go func() {
		timer := time.NewTimer(resetDuration)
		defer timer.Stop()

		<-timer.C
		close(r.throttle)
	}()
}

// AddResponse processes the HTTP response headers for rate limiting.
func (r *githubHeaderRateLimiter) AddResponse(resp *http.Response) {
	r.responses <- resp
}
