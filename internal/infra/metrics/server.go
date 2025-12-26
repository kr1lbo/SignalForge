package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server serves Prometheus metrics over HTTP
type Server struct {
	logger *slog.Logger
	server *http.Server
	port   int
}

// NewServer creates a new metrics HTTP server
func NewServer(logger *slog.Logger, port int) *Server {
	return &Server{
		logger: logger.With("component", "metrics_server"),
		port:   port,
	}
}

// Start begins serving metrics
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.logger.Info("metrics server starting", "port", s.port,
		"metrics_url", fmt.Sprintf("http://localhost:%d/metrics", s.port))

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server error: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("metrics server stopping")
	return s.server.Shutdown(ctx)
}
