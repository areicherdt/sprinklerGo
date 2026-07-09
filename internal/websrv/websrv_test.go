package websrv

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func get(t *testing.T, port int) (int, string) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func newTestServer(t *testing.T) (*Server, int) {
	t.Helper()
	s := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	}))
	port := freePort(t)
	if err := s.Start(port); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		s.Shutdown(ctx)
	})
	return s, port
}

func TestStartAndServe(t *testing.T) {
	_, port := newTestServer(t)
	if code, body := get(t, port); code != 200 || body != "pong" {
		t.Fatalf("got %d %q", code, body)
	}
}

func TestSwapMovesToNewPort(t *testing.T) {
	s, oldPort := newTestServer(t)
	newPort := freePort(t)

	if err := s.Swap(newPort); err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, newPort); code != 200 || body != "pong" {
		t.Fatalf("new port: %d %q", code, body)
	}

	// The old listener goes away once its graceful shutdown finished.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if code, _ := get(t, oldPort); code == 0 {
			return // connection refused — old server is gone
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("old port still accepting connections")
}

func TestSwapToOccupiedPortKeepsOldServer(t *testing.T) {
	s, oldPort := newTestServer(t)

	// Occupy a port, then try to swap onto it.
	blocker, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	busy := blocker.Addr().(*net.TCPAddr).Port

	if err := s.Swap(busy); err == nil {
		t.Fatal("swap to occupied port must fail")
	}
	// The old server keeps serving.
	if code, _ := get(t, oldPort); code != 200 {
		t.Fatalf("old server gone after failed swap: %d", code)
	}
}

func TestSwapToSamePortIsNoop(t *testing.T) {
	s, port := newTestServer(t)
	if err := s.Swap(port); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, port); code != 200 {
		t.Fatal("server must keep serving after same-port swap")
	}
}
