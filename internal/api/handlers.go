package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

// maxBodyBytes caps request bodies so a malicious client cannot exhause memory
const maxBodyBytes = 1 << 20 // 1 MiB

// handleHealthz reports that the process is alive. It touches no depndencies,
// so a restart loop caused by a flaky database is impossible.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz reports whether the service can actually serve traffic, which
// means its database is reachable.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.ready == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.ready.Ping(ctx); err != nil {
		loggerFrom(ctx).Warn("readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "database unreachable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleCreateService registers a service
func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	var req createServiceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	svc, err := catalog.New(catalog.NewServiceInput{
		Name:        req.Name,
		Description: req.Description,
		OwnerTeam:   req.OwnerTeam,
		RepoURL:     req.RepoURL,
		Tier:        req.Tier,
	}, s.now())
	if err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	if err := s.store.CreateService(r.Context(), svc); err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/services/"+svc.ID.String())
	writeJSON(w, http.StatusCreated, toServiceResponse(svc, scorecard.Report{}, false))
}

// handleListServices returns the catalog with each service's latest scorecard.
func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	services, err := s.store.ListServices(ctx)
	if err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	out := make([]serviceResponse, 0, len(services))
	for _, svc := range services {
		report, found, err := s.store.GetReport(ctx, svc.ID)
		if err != nil {
			writeDomainError(r.Context(), w, err)
			return
		}
		out = append(out, toServiceResponse(svc, report, found))
	}
	writeJSON(w, http.StatusOK, listServicesResponse{Services: out})
}

// handleGetService returns one service plus its scorecard breakdown.
func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {

	id, ok := parseIDParam(w, r)

	if !ok {
		return
	}

	ctx := r.Context()

	svc, err := s.store.GetService(ctx, id)
	if err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	report, found, err := s.store.GetReport(ctx, id)
	if err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceResponse(svc, report, found))
}

// handleDeleteService removes a service.
func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	if err := s.store.DeleteService(r.Context(), id); err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleEvaluateService gathers repository facts, scores them, and persists
// the result.
func (s *Server) handleEvaluateService(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	svc, err := s.store.GetService(ctx, id)
	if err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	facts, err := s.fetcher.Fetch(ctx, svc.RepoURL)
	if err != nil {
		if errors.Is(err, catalog.ErrRepoUnreadable) {
			evaluationsTotal.WithLabelValues("repo_unreadable").Inc()
			writeError(w, http.StatusUnprocessableEntity,
				"repository could not be read: check repo_url, or the repository may be "+
					"private and require GITHUB_TOKEN")
			return
		}
		evaluationsTotal.WithLabelValues("fetch_failed").Inc()
		loggerFrom(ctx).Error("fetching repository facts", "service", svc.Name, "repo_url", svc.RepoURL, "error", err)
		writeError(w, http.StatusBadGateway, "could not gather repository facts")
		return
	}

	// Ownership is a catalog fact, not a repository fact.
	facts.OwnerTeam = svc.OwnerTeam

	report := scorecard.Evaluate(facts)

	if err := s.store.SaveReport(ctx, id, report); err != nil {
		writeDomainError(r.Context(), w, err)
		return
	}

	evaluationsTotal.WithLabelValues("success").Inc()
	evaluationScore.Observe(float64(report.OverallScore))

	writeJSON(w, http.StatusOK, toServiceResponse(svc, report, true))
}

// handleListChecks return the check definitions and their weights.
func (s *Server) handleListChecks(w http.ResponseWriter, r *http.Request) {
	out := make([]checkDefinitionResponse, 0, len(scorecard.Checks))
	for _, c := range scorecard.Checks {
		out = append(out, checkDefinitionResponse{
			Key:         c.Key,
			Description: c.Description,
			Weight:      c.Weight,
		})
	}
	writeJSON(w, http.StatusOK, listChecksResponse{Checks: out})
}

// parseIDParam extracts and validates the {id} path segment. It writes an
// error response and reports false if the value is not a UUID.
func parseIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := r.PathValue("id")

	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a UUID")
		return uuid.UUID{}, false
	}

	return id, true
}

// decodeJSON reads exactly one JSON object into dst.  It writes an error
// response and reports false on any problem.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, "request body is empty")
		case errors.As(err, &maxErr):
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		default:
			writeError(w, http.StatusBadRequest, "malformed JSON: "+err.Error())
		}
		return false
	}

	// Reject trailing content, e.g. two JSON objects in one body.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "body must contain a single JSON object")
		return false
	}
	return true
}
