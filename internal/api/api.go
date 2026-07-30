// Package api exposes the catalog over HTTP.  It owns request parsing, JSON
// shapes, and status codes; it delegates all logic to the domain packages.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

// Store is the persistence this API needs.  It is declared here, in the
// consumer, so the API depends on a description of what it requires rather
// than on any particular database.
type Store interface {
	CreateService(ctx context.Context, svc catalog.Service) error
	GetService(ctx context.Context, id uuid.UUID) (catalog.Service, error)
	ListServices(ctx context.Context) ([]catalog.Service, error)
	DeleteService(ctx context.Context, id uuid.UUID) error
	SaveReport(ctx context.Context, serviceID uuid.UUID, r scorecard.Report) error
	GetReport(ctx context.Context, serviceID uuid.UUID) (scorecard.Report, bool, error)
}

// FactFetcher gathers facts about a repository.
type FactFetcher interface {
	Fetch(ctx context.Context, repoURL string) (scorecard.Facts, error)
}

// Server holds the API's dependencies.
type Server struct {
	store   Store
	fetcher FactFetcher

	// now is injected so handlers that stamp timestamps stay testable.
	now func() time.Time
}

// NewServer wires an API server.
func NewServer(store Store, fetcher FactFetcher) *Server {
	return &Server{
		store:   store,
		fetcher: fetcher,
		now:     time.Now,
	}
}

// Routes returns the fully-onfigured HTTP handler
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /api/v1/services", s.handleCreateService)
	mux.HandleFunc("GET /api/v1/services", s.handleListServices)
	mux.HandleFunc("GET /api/v1/services/{id}", s.handleGetService)
	mux.HandleFunc("DELETE /api/v1/services/{id}", s.handleDeleteService)
	mux.HandleFunc("POST /api/v1/services/{id}/evaluate", s.handleEvaluateService)

	mux.HandleFunc("GET /api/v1/checks", s.handleListChecks)

	return mux
}
