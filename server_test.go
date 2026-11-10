package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func start(t *testing.T, h http.Handler, timeout time.Duration) (url string, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- runServer(ctx, ln, h, timeout) }()
	t.Cleanup(cancel)
	return "http://" + ln.Addr().String(), cancel, errc
}

func waitDone(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not return")
		return nil
	}
}

func TestRunServerServesAndStopsCleanly(t *testing.T) {
	url, cancel, done := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}), time.Second)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}

	cancel()
	if err := waitDone(t, done); err != nil {
		t.Errorf("clean shutdown returned %v, want nil", err)
	}

	if _, err := http.Get(url); err == nil {
		t.Error("server still accepting connections after shutdown")
	}
}

func TestRunServerLetsInFlightRequestsFinish(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	url, cancel, done := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, "finished")
	}), 5*time.Second)

	result := make(chan string, 1)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			result <- "error: " + err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		result <- string(b)
	}()

	<-started
	cancel()
	// Shutdown is now waiting on the in-flight request.
	time.Sleep(50 * time.Millisecond)
	close(release)

	if got := <-result; got != "finished" {
		t.Errorf("in-flight request got %q, want finished", got)
	}
	if err := waitDone(t, done); err != nil {
		t.Errorf("runServer returned %v, want nil", err)
	}
}

func TestRunServerClosesConnectionsAfterTimeout(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	url, cancel, done := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block // never finishes on its own
	}), 50*time.Millisecond)

	clientErr := make(chan error, 1)
	go func() {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
		}
		clientErr <- err
	}()
	<-started
	cancel()

	err := waitDone(t, done)
	if !errors.Is(err, errShutdownTimedOut) || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("runServer returned %v, want errShutdownTimedOut wrapping context.DeadlineExceeded", err)
	}

	// The stuck request's connection must be closed, not left hanging.
	select {
	case err := <-clientErr:
		if err == nil {
			t.Error("stuck request completed; want its connection closed")
		}
	case <-time.After(2 * time.Second):
		t.Error("stuck request's connection was left open after shutdown")
	}
}

func TestRunServerReturnsServeErrors(t *testing.T) {
	ln := listen(t)
	ln.Close()

	err := runServer(context.Background(), ln, http.NotFoundHandler(), time.Second)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Errorf("runServer on a closed listener returned %v, want a real error", err)
	}
}

// TestBinaryExitsZeroOnSIGTERM builds the real app and checks that it exits
// cleanly when stopped the way Railway stops a container during a redeploy.
func TestBinaryExitsZeroOnSIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM is not supported on Windows")
	}
	if testing.Short() {
		t.Skip("builds the binary")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "datatrader")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	for _, stalledRequest := range []bool{false, true} {
		name := "idle"
		if stalledRequest {
			name = "shutdown timeout"
		}
		t.Run(name, func(t *testing.T) {
			testBinaryExitsZeroOnSIGTERM(t, bin, stalledRequest)
		})
	}
}

func testBinaryExitsZeroOnSIGTERM(t *testing.T, bin string, stalledRequest bool) {
	t.Helper()
	dir := t.TempDir()
	ln := listen(t)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cmd := exec.Command(bin)
	cmd.Dir = dir // no .env here, like production
	cmd.Env = append(os.Environ(), "DB_PATH="+filepath.Join(dir, "test.db"), "PORT="+strconv.Itoa(port))
	var logs strings.Builder
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait exactly once, in the background, so every path can both observe the
	// exit and make sure the child is reaped before reading its logs. Closing
	// exited lets cleanup wait again without consuming the result.
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	stopChild := func() {
		cmd.Process.Kill()
		<-exited
	}
	t.Cleanup(stopChild)

	// Each readiness probe is bounded, so a server that accepts but never
	// answers cannot stall the loop past its deadline.
	client := &http.Client{Timeout: time.Second}
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/login"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			break
		}
		select {
		case <-exited:
			t.Fatalf("server exited before it was ready (%v)\nlogs:\n%s", waitErr, logs.String())
		default:
		}
		if ctx.Err() != nil {
			stopChild()
			t.Fatalf("server never came up: %v\nlogs:\n%s", err, logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	if stalledRequest {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		// The login handler reads form data. Hold its body open, and wait for
		// 100 Continue to prove the handler is reading before sending SIGTERM.
		_, err = io.WriteString(conn, "POST /api/auth/login HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 100\r\nExpect: 100-continue\r\n\r\n")
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusContinue {
			t.Fatalf("status = %d, want 100 Continue", resp.StatusCode)
		}
		if err := conn.SetDeadline(time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
		if waitErr != nil {
			t.Fatalf("process exited with %v, want exit code 0\nlogs:\n%s", waitErr, logs.String())
		}
	case <-time.After(10 * time.Second):
		stopChild()
		t.Fatalf("process did not exit after SIGTERM\nlogs:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "HTTP server stopped") {
		t.Errorf("expected a clean-stop log line, got:\n%s", logs.String())
	}
	if stalledRequest && !strings.Contains(logs.String(), "shutdown grace period expired") {
		t.Errorf("expected a shutdown-timeout warning, got:\n%s", logs.String())
	}
}
