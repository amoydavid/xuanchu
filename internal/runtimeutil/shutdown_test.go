package runtimeutil

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownCoordinatorBeginRunningSucceedsAndDrainReturnsAfterDone(t *testing.T) {
	coordinator := NewShutdownCoordinator()

	done, ok := coordinator.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}
	if !coordinator.Accepting() {
		t.Fatal("Accepting() = false, want true")
	}

	done()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := coordinator.Drain(ctx); err != nil {
		t.Fatalf("Drain() error = %v, want nil", err)
	}
	if coordinator.Accepting() {
		t.Fatal("Accepting() = true after Drain, want false")
	}
	if !coordinator.IsDraining() {
		t.Fatal("IsDraining() = false after Drain, want true")
	}
}

func TestShutdownCoordinatorStopAcceptingMakesBeginFail(t *testing.T) {
	coordinator := NewShutdownCoordinator()

	coordinator.StopAccepting()

	if _, ok := coordinator.Begin(); ok {
		t.Fatal("Begin() ok = true after StopAccepting, want false")
	}
	if coordinator.Accepting() {
		t.Fatal("Accepting() = true after StopAccepting, want false")
	}
}

func TestShutdownCoordinatorDrainWaitsForActiveWork(t *testing.T) {
	coordinator := NewShutdownCoordinator()
	done, ok := coordinator.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}

	drained := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		drained <- coordinator.Drain(ctx)
	}()

	select {
	case err := <-drained:
		t.Fatalf("Drain() returned before done(): %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	done()

	select {
	case err := <-drained:
		if err != nil {
			t.Fatalf("Drain() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Drain() did not return after done()")
	}
}

func TestShutdownCoordinatorDrainReturnsDeadlineExceeded(t *testing.T) {
	coordinator := NewShutdownCoordinator()
	_, ok := coordinator.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := coordinator.Drain(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestShutdownCoordinatorForceCancelCancelsContext(t *testing.T) {
	coordinator := NewShutdownCoordinator()

	coordinator.ForceCancel()
	coordinator.ForceCancel()

	select {
	case <-coordinator.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("Context() was not canceled by ForceCancel()")
	}
}

func TestShutdownCoordinatorDoneIsIdempotent(t *testing.T) {
	coordinator := NewShutdownCoordinator()
	done, ok := coordinator.Begin()
	if !ok {
		t.Fatal("Begin() ok = false, want true")
	}

	done()
	done()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := coordinator.Drain(ctx); err != nil {
		t.Fatalf("Drain() error = %v, want nil", err)
	}
}
