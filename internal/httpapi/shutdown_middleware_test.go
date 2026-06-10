package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
)

func TestShutdownMiddlewareRejectsNewRequestsWhenDraining(t *testing.T) {
	shutdown := runtimeutil.NewShutdownCoordinator()
	shutdown.StopAccepting()
	srv := NewServer(Options{
		Store:    openHTTPTestStore(t),
		Shutdown: shutdown,
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Router().ServeHTTP(rr, req)

	assertHTTPErrorCode(t, rr, http.StatusServiceUnavailable, "server_draining")
}

func TestShutdownMiddlewareTracksInflightRequests(t *testing.T) {
	shutdown := runtimeutil.NewShutdownCoordinator()
	srv := NewServer(Options{
		Store:    openHTTPTestStore(t),
		Shutdown: shutdown,
	})

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	handler := srv.shutdownMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		close(done)
	}))

	go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))
	<-started

	drained := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		drained <- shutdown.Drain(ctx)
	}()

	select {
	case err := <-drained:
		t.Fatalf("Drain returned before request completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	close(release)
	<-done
	if err := <-drained; err != nil {
		t.Fatalf("Drain() error = %v", err)
	}
}

func TestShutdownMiddlewareForceCancelCancelsRequestContext(t *testing.T) {
	shutdown := runtimeutil.NewShutdownCoordinator()
	srv := NewServer(Options{
		Store:    openHTTPTestStore(t),
		Shutdown: shutdown,
	})

	started := make(chan struct{})
	canceled := make(chan struct{})
	handler := srv.shutdownMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))

	go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))
	<-started
	shutdown.ForceCancel()

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("request context was not canceled after ForceCancel()")
	}
}
