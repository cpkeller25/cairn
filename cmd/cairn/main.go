package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/cpkeller25/cairn/internal/api"
	"github.com/cpkeller25/cairn/internal/ingest"
	"github.com/cpkeller25/cairn/internal/store"
)

func main() {
	st := store.NewMemoryStore()
	fetcher := ingest.NewStubFetcher()
	apiServer := api.NewServer(st, fetcher)

	srv := &http.Server{
		Handler:      apiServer.Routes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	addr := ":" + port()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("cannot listen on %s: %v", addr, err)
	}

	log.Printf("cairn listening on %s", ln.Addr())

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}
