package ghinstallation

import (
	"context"
	"sync"
)

// semaphore type limits concurrency
type semaphore chan struct{}

// acquire tries to acquire a slot in the semaphore with context support.
// If the context expires before a slot is acquired, it returns an error.
func (s semaphore) acquire(ctx context.Context) error {
	select {
	case s <- struct{}{}:
		// Acquired a slot
		return nil
	case <-ctx.Done():
		// Context expired or was cancelled
		return ctx.Err()
	}
}

// release a slot in the semaphore
func (s semaphore) release() {
	<-s
}

// newSemaphore creates a new semaphore with a given capacity
func newSemaphore(length int) semaphore {
	return make(semaphore, length)
}

// semaphoreMap manages semaphores for different installations
type semaphoreMap struct {
	semaphores map[int64]semaphore // map of installation ID to semaphores
	mu         sync.Mutex          // mutex to protect the map
	limit      int                 // concurrency limit per semaphore
}

// newSemaphoreMap creates a semaphoreMap with a given concurrency limit
func newSemaphoreMap(limit int) *semaphoreMap {
	return &semaphoreMap{
		semaphores: make(map[int64]semaphore),
		limit:      limit,
	}
}

// getSemaphore returns a semaphore for a given installation ID, initializing it if necessary
func (sm *semaphoreMap) getSemaphore(installationID int64) semaphore {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if semaphore already exists
	if sem, exists := sm.semaphores[installationID]; exists {
		return sem
	}

	// Initialize new semaphore if it doesn't exist
	newSem := newSemaphore(sm.limit)
	sm.semaphores[installationID] = newSem
	return newSem
}
