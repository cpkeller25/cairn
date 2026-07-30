// Package store holds persistence adapters for the catalog.
package store

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

// MemoryStore keeps the catalog in process memory.  It is safe for concurrent
// use and exists so the API can be build and tested without a datbase
type MemoryStore struct {
	mu       sync.RWMutex
	services map[uuid.UUID]catalog.Service
	reports  map[uuid.UUID]scorecard.Report
}

// NewMemoryStore() returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		services: make(map[uuid.UUID]catalog.Service),
		reports:  make(map[uuid.UUID]scorecard.Report),
	}
}

// CreateService stores a new service, rejecting duplicate names.
func (s *MemoryStore) CreateService(ctx context.Context, svc catalog.Service) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.services {
		if existing.Name == svc.Name {
			return catalog.ErrNameTaken
		}
	}

	s.services[svc.ID] = svc
	return nil
}

// GetService returns the service with the given ID, or catalog.ErrNotFound
func (s *MemoryStore) GetService(ctx context.Context, id uuid.UUID) (catalog.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	svc, ok := s.services[id]
	if !ok {
		return catalog.Service{}, catalog.ErrNotFound
	}
	return svc, nil
}

// ListServices returns every service, ordered by name for stable output.
func (s *MemoryStore) ListServices(ctx context.Context) ([]catalog.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]catalog.Service, 0, len(s.services))
	for _, svc := range s.services {
		out = append(out, svc)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// DeleteService removes a service and any report it owns.
func (s *MemoryStore) DeleteService(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.services[id]; !ok {
		return catalog.ErrNotFound
	}
	delete(s.services, id)
	delete(s.reports, id)
	return nil
}

// SaveReport records teh latest scorecard for a service, replacing any prior
// result
func (s *MemoryStore) SaveReport(ctx context.Context, serviceID uuid.UUID, r scorecard.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.services[serviceID]; !ok {
		return catalog.ErrNotFound
	}
	s.reports[serviceID] = cloneReport(r)
	return nil
}

// GetReport returns the latest scorecard for a service.  The boolean reports
// whether a scorecard exists; a service that has never been evaluated is not
// an error.
func (s *MemoryStore) GetReport(ctx context.Context, serviceID uuid.UUID) (scorecard.Report, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.services[serviceID]; !ok {
		return scorecard.Report{}, false, catalog.ErrNotFound
	}

	r, ok := s.reports[serviceID]
	if !ok {
		return scorecard.Report{}, false, nil
	}
	return cloneReport(r), true, nil
}

// cloneReport deep-copies a Report so callers cannot mutate stored state
// through the Results slice
func cloneReport(r scorecard.Report) scorecard.Report {
	out := r
	out.Results = make([]scorecard.CheckResult, len(r.Results))
	copy(out.Results, r.Results)
	return out
}
