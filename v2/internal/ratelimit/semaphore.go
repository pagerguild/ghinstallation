package ratelimit

import (
	"context"
)

// Semaphore type limits concurrency
type Semaphore chan struct{}

// Acquire tries to Acquire a slot in the semaphore with context support.
// If the context expires before a slot is acquired, it returns an error.
func (s Semaphore) Acquire(ctx context.Context) error {
	select {
	case s <- struct{}{}:
		// Acquired a slot
		return nil
	case <-ctx.Done():
		// Context expired or was cancelled
		return ctx.Err()
	}
}

// Release a slot in the semaphore
func (s Semaphore) Release() {
	<-s
}

// newSemaphore creates a new semaphore with a given capacity
func newSemaphore(length int) Semaphore {
	return make(Semaphore, length)
}
