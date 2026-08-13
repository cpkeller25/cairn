package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/cpkeller25/cairn/internal/api"
	"github.com/cpkeller25/cairn/internal/config"
	"github.com/cpkeller25/cairn/internal/ingest"
	"github.com/cpkeller25/cairn/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run holds startup logic so every failure path can return an error rather
// than calling os.Exit, which would skip deferred cleanup.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.Migrate(pool); err != nil {
		return err
	}
	log.Printf("migrations up to date")

	// The composition root: the only place that names concrete adapters.
	st := store.NewPostgresStore(pool)
	fetcher := ingest.NewStubFetcher()
	apiServer := api.NewServer(st, fetcher)

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

	log.Printf("cairn listening on %s", ln.Addr())

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
