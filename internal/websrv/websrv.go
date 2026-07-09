// Package websrv runs the HTTP server and can move it to a new port at
// runtime, so a web-port change in the settings applies without restarting
// the service. Binding errors (occupied or privileged ports) surface
// synchronously to the caller instead of crashing a restarted daemon.
package websrv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	handler http.Handler
	errCh   chan error

	mu   sync.Mutex
	srv  *http.Server
	ln   net.Listener
	port int
}

func New(handler http.Handler) *Server {
	return &Server{handler: handler, errCh: make(chan error, 1)}
}

// Err delivers a fatal serve error (never ErrServerClosed).
func (s *Server) Err() <-chan error { return s.errCh }

// Addr returns the address the server currently listens on.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

func (s *Server) serve(srv *http.Server, ln net.Listener) {
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case s.errCh <- err:
			default:
			}
		}
	}()
}

// Start binds the port and begins serving. Non-blocking.
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	s.srv = &http.Server{Handler: s.handler}
	s.ln = ln
	s.port = port
	s.serve(s.srv, ln)
	return nil
}

// Swap moves the server to a new port: the new listener starts serving
// immediately (so the error path is synchronous), the old one shuts down
// gracefully in the background — in-flight responses, including the settings
// request that triggered the swap, still complete.
func (s *Server) Swap(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if port == s.port {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	newSrv := &http.Server{Handler: s.handler}
	s.serve(newSrv, ln)

	old := s.srv
	s.srv = newSrv
	s.ln = ln
	s.port = port

	if old != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Graceful first (lets active requests finish), then hard close
			// for long-lived connections such as the SSE streams.
			if err := old.Shutdown(ctx); err != nil {
				old.Close()
			}
			slog.Info("web server moved", "port", port)
		}()
	}
	return nil
}

// Shutdown stops the current server gracefully.
func (s *Server) Shutdown(ctx context.Context) {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv != nil {
		srv.Shutdown(ctx)
	}
}
