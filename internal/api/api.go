// Package api exposes the catalog over HTTP.  It owns request parsing, JSON
// shapes, and status codes; it delegates all logic to the domain packages.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

// ReadyChecker reports whether a dependency is usable right now.
// *pgxpool.Pool statisfies this.
type ReadyChecker interface {
	Ping(ctx context.Context) error
}

// Server holds the API's dependencies.
type Server struct {
	store   Store
	fetcher FactFetcher
	logger  *slog.Logger
	ready   ReadyChecker
	// now is injected so handlers that stamp timestamps stay testable.
	now func() time.Time
}

// ServerOption customises a Server.
type ServerOption func(*Server)

// WithLogger sets the logger. Defaults to slog.Default().
func WithLogger(l *slog.Logger) ServerOption {
	return func(s *Server) { s.logger = l }
}

// WithReadyChecker supplies the dependency probed by /readyz.
func WithReadyChecker(r ReadyChecker) ServerOption {
	return func(s *Server) { s.ready = r }
}

// NewServer wires an API server.
func NewServer(store Store, fetcher FactFetcher, opts ...ServerOption) *Server {
	s := &Server{
		store:   store,
		fetcher: fetcher,
		logger:  slog.Default(),
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Routes returns the fully-onfigured HTTP handler
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("POST /api/v1/services", s.handleCreateService)
	mux.HandleFunc("GET /api/v1/services", s.handleListServices)
	mux.HandleFunc("GET /api/v1/services/{id}", s.handleGetService)
	mux.HandleFunc("DELETE /api/v1/services/{id}", s.handleDeleteService)
	mux.HandleFunc("POST /api/v1/services/{id}/evaluate", s.handleEvaluateService)

	mux.HandleFunc("GET /api/v1/checks", s.handleListChecks)

	// Outermost first. requestID runs before logRequests so the log line
	// carries the ID; recoverPanic sits inside logRequests so a panic is
	// still recorded with its 500 status
	return chain(mux,
		s.requestID,
		s.recordMetrics,
		s.logRequests,
		s.recoverPanic,
	)
}
