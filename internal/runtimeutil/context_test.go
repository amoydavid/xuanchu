package runtimeutil

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestContextWithCancelOnEitherParentCancel(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	other, cancelOther := context.WithCancel(context.Background())
	defer cancelOther()

	ctx, cancel := ContextWithCancelOnEither(parent, other)
	defer cancel()

	cancelParent()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("ctx was not canceled after parent cancel")
	}
}

func TestContextWithCancelOnEitherOtherCancel(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	other, cancelOther := context.WithCancel(context.Background())

	ctx, cancel := ContextWithCancelOnEither(parent, other)
	defer cancel()

	cancelOther()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("ctx was not canceled after other cancel")
	}
}

func TestContextWithCancelOnEitherManualCancel(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	other, cancelOther := context.WithCancel(context.Background())
	defer cancelOther()

	ctx, cancel := ContextWithCancelOnEither(parent, other)
	cancel()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("ctx was not canceled after manual cancel")
	}
}

func TestContextWithCancelOnEitherNilOtherBehavesLikeWithCancel(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	ctx, cancel := ContextWithCancelOnEither(parent, nil)
	defer cancel()

	cancelParent()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("ctx was not canceled after parent cancel")
	}
}

func TestContextWithCancelOnEitherCancelReleasesWatcher(t *testing.T) {
	baseline := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		parent, cancelParent := context.WithCancel(context.Background())
		other, cancelOther := context.WithCancel(context.Background())
		ctx, cancel := ContextWithCancelOnEither(parent, other)
		cancel()
		<-ctx.Done()
		cancelParent()
		defer cancelOther()
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		if runtime.NumGoroutine() <= baseline+10 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("manual cancel appears to leak watcher goroutines: baseline=%d current=%d", baseline, runtime.NumGoroutine())
}
