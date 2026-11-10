package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// shutdownTimeout is how long in-flight requests get to finish once the
// process is asked to stop. Railway waits a few seconds before force-killing.
const shutdownTimeout = 5 * time.Second

// runServer serves h on ln until ctx is cancelled, then stops accepting new
// connections and waits up to timeout for in-flight requests to finish.
// It returns nil on a clean shutdown so the process can exit 0.
func runServer(ctx context.Context, ln net.Listener, h http.Handler, timeout time.Duration) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		// The server stopped on its own, which only happens on a real error.
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
