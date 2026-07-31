package app

import (
	"context"
	"net"
	"net/http"
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
