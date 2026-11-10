package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// shutdownTimeout is how long in-flight requests get to finish once the
// process is asked to stop. Railway waits a few seconds before force-killing.
const shutdownTimeout = 5 * time.Second

// errShutdownTimedOut means a requested shutdown went ahead, but some requests
// were still running when the grace period ran out and were cut off.
var errShutdownTimedOut = errors.New("shutdown grace period expired; remaining connections were closed")

// runServer serves h on ln until ctx is cancelled, then stops accepting new
// connections and waits up to timeout for in-flight requests to finish.
// It returns nil on a clean shutdown, errShutdownTimedOut (wrapped) if
// requests had to be cut off, and any other error if serving itself failed.
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
	shutdownErr := srv.Shutdown(shutdownCtx)
	var closeErr error
	if shutdownErr != nil {
		// Shutdown gives up waiting but leaves slow connections open; close them.
		closeErr = srv.Close()
	}
	err := <-serveErr
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if err != nil || closeErr != nil {
		return errors.Join(shutdownErr, closeErr, err)
	}
	if errors.Is(shutdownErr, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", errShutdownTimedOut, shutdownErr)
	}
	return shutdownErr
}
