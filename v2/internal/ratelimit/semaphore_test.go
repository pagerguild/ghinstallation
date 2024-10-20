package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestSemaphoreAcquire(t *testing.T) {
	sem := NewSemaphore(1)

	// Test acquiring a slot
	ctx := context.Background()
	err := sem.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected to acquire a slot, got error: %v", err)
	}

	// Test acquiring a slot with a full semaphore
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = sem.Acquire(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestSemaphoreRelease(t *testing.T) {
	sem := NewSemaphore(1)

	// Acquire a slot and then release it
	ctx := context.Background()
	err := sem.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected to acquire a slot, got error: %v", err)
	}

	sem.Release()

	// Acquire again to ensure the slot was released
	err = sem.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected to acquire a slot after release, got error: %v", err)
	}
}

func TestSemaphoreWithContextTimeout(t *testing.T) {
	sem := NewSemaphore(1)

	// Acquire the slot to fill the semaphore
	ctx := context.Background()
	err := sem.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected to acquire a slot, got error: %v", err)
	}

	// Try to acquire another slot, expect timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err = sem.Acquire(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded error, got: %v", err)
	}
}

func TestSemaphoreAcquireAfterRelease(t *testing.T) {
	sem := NewSemaphore(1)

	// Acquire and release multiple times to ensure proper functioning
	for i := 0; i < 3; i++ {
		ctx := context.Background()
		err := sem.Acquire(ctx)
		if err != nil {
			t.Fatalf("expected to acquire a slot on iteration %d, got error: %v", i, err)
		}
		sem.Release()
	}
}
