package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cpkeller25/cairn/internal/api"
	"github.com/cpkeller25/cairn/internal/config"
	"github.com/cpkeller25/cairn/internal/ingest"
	"github.com/cpkeller25/cairn/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

// run holds startup logic so every failure path can return an error rather
// than calling os.Exit, which would skip deferred cleanup.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// Cancelled on SIGINT or SIGTERM, which starts the shutdown sequence.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.Migrate(pool); err != nil {
		return err
	}
	logger.Info("migrations up to date")

	// The composition root: the only place that names concrete adapters.
	st := store.NewPostgresStore(pool)
	fetcher := ingest.NewGitHubFetcher(cfg.GitHubToken)
	apiServer := api.NewServer(st, fetcher,
		api.WithLogger(logger),
		api.WithReadyChecker(pool),
	)

	srv := &http.Server{
		Handler:      apiServer.Routes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		return err
	}

	// Serve in the background so main can wait on the shutdown signal.
	serveErr := make(chan error, 1)

	go func() {
		logger.Info("cairn listening", "addr", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	// Give in-flight requests up to 15 seconds to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		return err
	}

	logger.Info("shutdown complete")
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}

	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
