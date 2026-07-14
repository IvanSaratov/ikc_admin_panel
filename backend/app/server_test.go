package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlerLifecycleStopsAdmissionsAndWaitsForActiveHandler(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	lifecycle := newHandlerLifecycle(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		lifecycle.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/active", nil))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("active handler did not start")
	}

	drained := lifecycle.stop()
	rejected := httptest.NewRecorder()
	lifecycle.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/late", nil))
	if rejected.Code != http.StatusServiceUnavailable {
		t.Fatalf("late request status = %d, want 503", rejected.Code)
	}
	select {
	case <-drained:
		t.Fatal("lifecycle drained while active handler was still running")
	default:
	}

	close(release)
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("active handler did not return")
	}
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("lifecycle did not report drained handlers")
	}
}

func TestServerCloseWaitsForActiveHandler(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	lifecycle := newHandlerLifecycle(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))
	server := &Server{httpServer: &http.Server{}, handlers: lifecycle}

	go lifecycle.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/active", nil))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("active handler did not start")
	}

	closed := make(chan error, 1)
	go func() { closed <- server.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before active handler: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after active handler")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown after Close: %v", err)
	}
}
