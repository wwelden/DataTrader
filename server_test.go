package main

import (
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

func TestRunServerGivesUpAfterTimeout(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	url, cancel, done := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
	}), 50*time.Millisecond)

	go http.Get(url)
	<-started
	cancel()

	if err := waitDone(t, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("runServer returned %v, want context.DeadlineExceeded", err)
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
	t.Cleanup(func() { cmd.Process.Kill() })

	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/login"
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never came up: %v\nlogs:\n%s", err, logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("process exited with %v, want exit code 0\nlogs:\n%s", err, logs.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("process did not exit after SIGTERM")
	}
	if !strings.Contains(logs.String(), "HTTP server stopped") {
		t.Errorf("expected a clean-stop log line, got:\n%s", logs.String())
	}
}
