package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunHTTPServerStopsAfterCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := &http.Server{Handler: http.NewServeMux()}

	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, server, listener, time.Second)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run server: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}

func TestReadinessRequiresWorkersAndDatabase(t *testing.T) {
	var workers atomic.Bool
	ready := readyWhenWorkersStarted(func(context.Context) error { return nil }, &workers)
	if err := ready(context.Background()); err == nil {
		t.Fatal("readiness succeeded before workers started")
	}
	workers.Store(true)
	if err := ready(context.Background()); err != nil {
		t.Fatalf("readiness failed after workers started: %v", err)
	}
	databaseFailure := readyWhenWorkersStarted(func(context.Context) error { return errors.New("database unavailable") }, &workers)
	if err := databaseFailure(context.Background()); err == nil {
		t.Fatal("readiness ignored database failure")
	}
}
