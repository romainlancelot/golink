// Package server manages the HTTP server lifecycle, including
// Pi-hole reverse proxying and graceful shutdown.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/romainlancelot/golink/internal/config"
	"github.com/romainlancelot/golink/internal/store"
)

const shutdownTimeout = 5 * time.Second

// Server wraps the HTTP server with graceful shutdown and Pi-hole proxying.
type Server struct {
	cfg     config.Config
	handler http.Handler
	store   *store.Store
	logger  *slog.Logger
}

// New creates a Server.
func New(cfg config.Config, goHandler http.Handler, s *store.Store, logger *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		handler: goHandler,
		store:   s,
		logger:  logger,
	}
}

// Run starts the server and blocks until a termination signal is received.
// It performs a graceful shutdown, saving links before exit.
func (s *Server) Run() error {
	targetURL, err := url.Parse(s.cfg.PiholeTarget)
	if err != nil {
		return fmt.Errorf("parse pihole target %q: %w", s.cfg.PiholeTarget, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := strings.Split(r.Host, ":")[0]
		if host == "go" || host == "go.local" {
			s.handler.ServeHTTP(w, r)
		} else {
			proxy.ServeHTTP(w, r)
		}
	})

	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting server", "addr", s.cfg.ListenAddr, "pihole", s.cfg.PiholeTarget)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		s.logger.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server failed: %w", err)
	}

	if err := s.store.Save(); err != nil {
		s.logger.Error("failed to save links during shutdown", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	s.logger.Info("server stopped")
	return nil
}
