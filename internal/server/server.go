package server

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Server wraps the standard http.Server with start/shutdown helpers.
type Server struct {
	httpSrv *http.Server
}

// New creates a Server listening on addr and routing through handler.
func New(addr string, handler http.Handler) *Server {
	return &Server{
		httpSrv: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// Start begins serving HTTP. It blocks until the server is closed.
// Returns nil on clean shutdown (ErrServerClosed is swallowed).
func (s *Server) Start() error {
	if err := s.httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully drains active connections within the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
