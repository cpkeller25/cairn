package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
	"github.com/cpkeller25/cairn/internal/store"
)

// Compile-time proof that MemoryStore satisfies the Store interface. If the
// interface and the implementation ever drift, this fails to build.
var _ Store = (*store.MemoryStore)(nil)

// ---------- test doubles ----------

// fakeFetcher returns canned facts or a canned error.
type fakeFetcher struct {
	facts scorecard.Facts
	err   error
}

func (f fakeFetcher) Fetch(ctx context.Context, repoURL string) (scorecard.Facts, error) {
	return f.facts, f.err
}

// brokenStore embeds a real Store and overrides selected methods to fail,
// so the 500 path can be exercised.
type brokenStore struct {
	Store
	listErr error
}

func (b brokenStore) ListServices(ctx context.Context) ([]catalog.Service, error) {
	if b.listErr != nil {
		return nil, b.listErr
	}
	return b.Store.ListServices(ctx)
}

// ---------- helpers ----------

var fixedNow = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	srv := NewServer(store.NewMemoryStore(), fakeFetcher{
		facts: scorecard.Facts{
			HasReadme:    true,
			HasCI:        true,
			HasTests:     true,
			LastCommitAt: fixedNow.AddDate(0, 0, -5),
			FetchedAt:    fixedNow,
		},
	})
	srv.now = func() time.Time { return fixedNow }
	return srv
}

// do issues a request against the server's routes and returns the recorder.
func do(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if raw, ok := body.(string); ok {
			buf.WriteString(raw)
		} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encoding request body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding response %q: %v", rec.Body.String(), err)
	}
	return out
}

func validBody() map[string]any {
	return map[string]any{
		"name":        "payments-api",
		"description": "Handles payments",
		"owner_team":  "checkout",
		"repo_url":    "https://github.com/acme/payments-api",
		"tier":        1,
	}
}

// createService registers a service and returns its ID.
func createService(t *testing.T, srv *Server, name string) string {
	t.Helper()

	body := validBody()
	body["name"] = name

	rec := do(t, srv, http.MethodPost, "/api/v1/services", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status %d, body %s", name, rec.Code, rec.Body)
	}
	return decode[serviceResponse](t, rec).ID
}

// ---------- create ----------

func TestCreateService(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodPost, "/api/v1/services", validBody())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	got := decode[serviceResponse](t, rec)

	if got.Name != "payments-api" {
		t.Errorf("Name = %q, want payments-api", got.Name)
	}
	if got.OwnerTeam != "checkout" {
		t.Errorf("OwnerTeam = %q, want checkout", got.OwnerTeam)
	}
	if got.Tier != 1 {
		t.Errorf("Tier = %d, want 1", got.Tier)
	}
	if got.Scorecard != nil {
		t.Error("Scorecard should be null before evaluation")
	}
	if _, err := uuid.Parse(got.ID); err != nil {
		t.Errorf("ID %q is not a UUID", got.ID)
	}
	if !got.CreatedAt.Equal(fixedNow) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, fixedNow)
	}
	if want := "/api/v1/services/" + got.ID; rec.Header().Get("Location") != want {
		t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), want)
	}
}

func TestCreateServiceDefaultsTier(t *testing.T) {
	srv := newTestServer(t)

	body := validBody()
	delete(body, "tier")

	rec := do(t, srv, http.MethodPost, "/api/v1/services", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := decode[serviceResponse](t, rec); got.Tier != catalog.DefaultTier {
		t.Errorf("Tier = %d, want %d", got.Tier, catalog.DefaultTier)
	}
}

func TestCreateServiceValidation(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(map[string]any)
		wantField string
	}{
		{"missing name", func(b map[string]any) { delete(b, "name") }, "name"},
		{"bad name format", func(b map[string]any) { b["name"] = "Payments API" }, "name"},
		{"missing owner", func(b map[string]any) { delete(b, "owner_team") }, "owner_team"},
		{"missing repo", func(b map[string]any) { delete(b, "repo_url") }, "repo_url"},
		{"bad repo scheme", func(b map[string]any) { b["repo_url"] = "ftp://x.com/a" }, "repo_url"},
		{"tier too high", func(b map[string]any) { b["tier"] = 9 }, "tier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			body := validBody()
			tt.mutate(body)

			rec := do(t, srv, http.MethodPost, "/api/v1/services", body)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body)
			}

			got := decode[errorResponse](t, rec)
			found := false
			for _, fe := range got.Fields {
				if fe.Field == tt.wantField {
					found = true
				}
			}
			if !found {
				t.Errorf("expected an error on %q, got %+v", tt.wantField, got.Fields)
			}
		})
	}
}

func TestCreateServiceDuplicateName(t *testing.T) {
	srv := newTestServer(t)
	createService(t, srv, "payments-api")

	rec := do(t, srv, http.MethodPost, "/api/v1/services", validBody())

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body)
	}
}

func TestCreateServiceBadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed json", `{"name": `, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
		{"unknown field", `{"name":"a","owner_teem":"x"}`, http.StatusBadRequest},
		{"two objects", `{"name":"a"}{"name":"b"}`, http.StatusBadRequest},
		{"not an object", `["a"]`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)

			rec := do(t, srv, http.MethodPost, "/api/v1/services", tt.body)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

// ---------- read ----------

func TestGetService(t *testing.T) {
	srv := newTestServer(t)
	id := createService(t, srv, "payments-api")

	rec := do(t, srv, http.MethodGet, "/api/v1/services/"+id, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	got := decode[serviceResponse](t, rec)
	if got.ID != id {
		t.Errorf("ID = %q, want %q", got.ID, id)
	}
	if got.Scorecard != nil {
		t.Error("Scorecard should be null before evaluation")
	}
}

func TestGetServiceNotFound(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodGet, "/api/v1/services/"+uuid.New().String(), nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetServiceBadID(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodGet, "/api/v1/services/not-a-uuid", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestListServicesEmpty(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodGet, "/api/v1/services", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decode[listServicesResponse](t, rec); len(got.Services) != 0 {
		t.Errorf("got %d services, want 0", len(got.Services))
	}
	// A nil slice would marshal as null and break clients that iterate.
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"services":[]`)) {
		t.Errorf("expected an empty array, got %s", rec.Body)
	}
}

func TestListServicesSorted(t *testing.T) {
	srv := newTestServer(t)
	createService(t, srv, "zebra-svc")
	createService(t, srv, "alpha-svc")

	rec := do(t, srv, http.MethodGet, "/api/v1/services", nil)

	got := decode[listServicesResponse](t, rec)
	if len(got.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(got.Services))
	}
	if got.Services[0].Name != "alpha-svc" {
		t.Errorf("first = %q, want alpha-svc", got.Services[0].Name)
	}
}

func TestListServicesStoreFailureIs500(t *testing.T) {
	srv := newTestServer(t)
	srv.store = brokenStore{
		Store:   srv.store,
		listErr: errors.New("database on fire"),
	}

	rec := do(t, srv, http.MethodGet, "/api/v1/services", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	// The internal detail must not reach the client.
	if bytes.Contains(rec.Body.Bytes(), []byte("on fire")) {
		t.Errorf("internal error leaked to client: %s", rec.Body)
	}
}

// ---------- evaluate ----------

func TestEvaluateService(t *testing.T) {
	srv := newTestServer(t)
	id := createService(t, srv, "payments-api")

	rec := do(t, srv, http.MethodPost, "/api/v1/services/"+id+"/evaluate", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	got := decode[serviceResponse](t, rec)
	if got.Scorecard == nil {
		t.Fatal("Scorecard is null after evaluation")
	}
	if len(got.Scorecard.Checks) != len(scorecard.Checks) {
		t.Errorf("got %d checks, want %d", len(got.Scorecard.Checks), len(scorecard.Checks))
	}
	if got.Scorecard.Level == "" {
		t.Error("Level is empty")
	}

	// has_owner must pass: ownership comes from the catalog, not the fetcher.
	for _, c := range got.Scorecard.Checks {
		if c.Key == "has_owner" && !c.Passed {
			t.Error("has_owner failed despite the service having an owning team")
		}
	}
}

func TestEvaluatePersistsResult(t *testing.T) {
	srv := newTestServer(t)
	id := createService(t, srv, "payments-api")

	evalRec := do(t, srv, http.MethodPost, "/api/v1/services/"+id+"/evaluate", nil)
	want := decode[serviceResponse](t, evalRec).Scorecard.OverallScore

	getRec := do(t, srv, http.MethodGet, "/api/v1/services/"+id, nil)
	got := decode[serviceResponse](t, getRec)

	if got.Scorecard == nil {
		t.Fatal("Scorecard is null on a subsequent GET")
	}
	if got.Scorecard.OverallScore != want {
		t.Errorf("persisted score = %d, want %d", got.Scorecard.OverallScore, want)
	}
}

func TestEvaluateMissingService(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodPost, "/api/v1/services/"+uuid.New().String()+"/evaluate", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestEvaluateFetcherFailureIs502(t *testing.T) {
	srv := newTestServer(t)
	id := createService(t, srv, "payments-api")
	srv.fetcher = fakeFetcher{err: errors.New("github timed out")}

	rec := do(t, srv, http.MethodPost, "/api/v1/services/"+id+"/evaluate", nil)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 (body: %s)", rec.Code, rec.Body)
	}
}

// ---------- delete ----------

func TestDeleteService(t *testing.T) {
	srv := newTestServer(t)
	id := createService(t, srv, "payments-api")

	rec := do(t, srv, http.MethodDelete, "/api/v1/services/"+id, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 must have an empty body, got %s", rec.Body)
	}

	if again := do(t, srv, http.MethodGet, "/api/v1/services/"+id, nil); again.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want 404", again.Code)
	}
}

func TestDeleteMissingService(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodDelete, "/api/v1/services/"+uuid.New().String(), nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ---------- checks & routing ----------

func TestListChecks(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodGet, "/api/v1/checks", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	got := decode[listChecksResponse](t, rec)
	if len(got.Checks) != len(scorecard.Checks) {
		t.Fatalf("got %d checks, want %d", len(got.Checks), len(scorecard.Checks))
	}

	total := 0
	for _, c := range got.Checks {
		if c.Key == "" || c.Description == "" {
			t.Errorf("incomplete check definition: %+v", c)
		}
		total += c.Weight
	}
	if total != 100 {
		t.Errorf("weights sum to %d, want 100", total)
	}
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRouting(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"wrong method on collection", http.MethodPut, "/api/v1/services", http.StatusMethodNotAllowed},
		{"wrong method on checks", http.MethodPost, "/api/v1/checks", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/api/v1/nope", http.StatusNotFound},
		{"unversioned path", http.MethodGet, "/services", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)

			rec := do(t, srv, tt.method, tt.path, nil)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRoutesUseUppercaseMethods(t *testing.T) {
	srv := newTestServer(t)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/api/v1/services"},
		{http.MethodGet, "/api/v1/checks"},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			if rec := do(t, srv, r.method, r.path, nil); rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("got 405 — is the method token in the route pattern uppercase?")
			}
		})
	}
}
