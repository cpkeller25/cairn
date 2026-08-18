package ingest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cpkeller25/cairn/internal/catalog"
)

var testClock = func() time.Time {
	return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
}

// fageGitHub serves canned responses for one repository
type fakeGitHub struct {
	repoJSON   string
	treeJSON   string
	repoStatus int
	treeStatus int
	headers    map[string]string

	// captured from the last request
	gotAuth  string
	gotPaths []string
}

func (f *fakeGitHub) server(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		f.gotAuth = r.Header.Get("Authorization")
		f.gotPaths = append(f.gotPaths, r.URL.Path)
		f.write(w, f.repoStatus, f.repoJSON)
	})

	mux.HandleFunc("GET /repos/{owner}/{repo}/git/trees/{branch}", func(w http.ResponseWriter, r *http.Request) {
		f.gotPaths = append(f.gotPaths, r.URL.Path+"?"+r.URL.RawQuery)
		f.write(w, f.treeStatus, f.treeJSON)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeGitHub) write(w http.ResponseWriter, status int, body string) {
	for k, v := range f.headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

const fullRepoJSON = `{
	"full_name": "acme/payments-api",
	"default_branch": "main",
	"pushed_at": "2026-08-14T09:30:00Z",
	"archived": false,
	"license": {"key": "mit", "name": "MIT License"}
}`

const fullTreeJSON = `{
	"sha": "abc123",
	"truncated": false,
	"tree": [
		{"path": "README.md", "type": "blob"},
		{"path": "LICENSE", "type": "blob"},
		{"path": "Dockerfile", "type": "blob"},
		{"path": ".github", "type": "tree"},
		{"path": ".github/workflows", "type": "tree"},
		{"path": ".github/workflows/ci.yml", "type": "blob"},
		{"path": "internal", "type": "tree"},
		{"path": "internal/pay/pay.go", "type": "blob"},
		{"path": "internal/pay/pay_test.go", "type": "blob"}
	]
}`

func TestFetchFullRepo(t *testing.T) {
	fake := &fakeGitHub{repoJSON: fullRepoJSON, treeJSON: fullTreeJSON}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))

	facts, err := f.Fetch(context.Background(), "https://github.com/acme/payments-api")
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if !facts.HasReadme {
		t.Error("HasReadme = false, want true")
	}
	if !facts.HasCI {
		t.Error("HasCI = false, want true")
	}
	if !facts.HasTests {
		t.Error("HasTests = false, want true")
	}
	if !facts.HasDockerfile {
		t.Error("HasDockerfile = false, want true")
	}
	if !facts.HasLicense {
		t.Error("HasLicense = false, want true")
	}

	wantCommit := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	if !facts.LastCommitAt.Equal(wantCommit) {
		t.Errorf("LastCommitAt = %v, want %v", facts.LastCommitAt, wantCommit)
	}
	if !facts.FetchedAt.Equal(testClock()) {
		t.Errorf("FetchedAt = %v, want %v", facts.FetchedAt, testClock())
	}
}

func TestFetchBareRepo(t *testing.T) {
	fake := &fakeGitHub{
		repoJSON: `{"default_branch":"main","pushed_at":"2020-01-01T00:00:00Z","license":null}`,
		treeJSON: `{"truncated":false,"tree":[{"path":"main.go","type":"blob"}]}`,
	}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))

	facts, err := f.Fetch(context.Background(), "github.com/acme/bare")
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if facts.HasReadme || facts.HasCI || facts.HasTests || facts.HasDockerfile || facts.HasLicense {
		t.Errorf("expected all facts false, got %+v", facts)
	}
}

func TestFetchUsesDefaultBranch(t *testing.T) {
	fake := &fakeGitHub{
		repoJSON: `{"default_branch":"trunk","pushed_at":"2026-01-01T00:00:00Z"}`,
		treeJSON: `{"tree":[]}`,
	}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))

	if _, err := f.Fetch(context.Background(), "github.com/acme/x"); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	found := false
	for _, p := range fake.gotPaths {
		if p == "/repos/acme/x/git/trees/trunk?recursive=1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a request for the trunk branch, got %v", fake.gotPaths)
	}
}

func TestFetchSendsAuthorizationWhenTokenSet(t *testing.T) {
	fake := &fakeGitHub{
		repoJSON: `{"default_branch":"main","pushed_at":"2026-01-01T00:00:00Z"}`,
		treeJSON: `{"tree":[]}`,
	}
	srv := fake.server(t)

	f := NewGitHubFetcher("secret-token", WithBaseURL(srv.URL), WithClock(testClock))
	if _, err := f.Fetch(context.Background(), "github.com/acme/x"); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if fake.gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want %q", fake.gotAuth, "Bearer secret-token")
	}
}

func TestFetchOmitsAuthorizationWhenNoToken(t *testing.T) {
	fake := &fakeGitHub{
		repoJSON: `{"default_branch":"main","pushed_at":"2026-01-01T00:00:00Z"}`,
		treeJSON: `{"tree":[]}`,
	}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))
	if _, err := f.Fetch(context.Background(), "github.com/acme/x"); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if fake.gotAuth != "" {
		t.Errorf("Authorization = %q, want it absent", fake.gotAuth)
	}
}

func TestFetchRepoNotFound(t *testing.T) {
	fake := &fakeGitHub{repoStatus: http.StatusNotFound, repoJSON: `{"message":"Not Found"}`}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))

	_, err := f.Fetch(context.Background(), "github.com/acme/ghost")
	if !errors.Is(err, catalog.ErrRepoUnreadable) {
		t.Errorf("err = %v, want catalog.ErrRepoUnreadable", err)
	}
}

func TestFetchRateLimited(t *testing.T) {
	fake := &fakeGitHub{
		repoStatus: http.StatusForbidden,
		repoJSON:   `{"message":"API rate limit exceeded"}`,
		headers:    map[string]string{"X-RateLimit-Remaining": "0"},
	}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))

	_, err := f.Fetch(context.Background(), "github.com/acme/x")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
	if errors.Is(err, catalog.ErrRepoUnreadable) {
		t.Error("rate limiting must not be reported as an unreadable repository")
	}
}

func TestFetchRejectsNonGitHubURL(t *testing.T) {
	f := NewGitHubFetcher("", WithClock(testClock))

	_, err := f.Fetch(context.Background(), "https://gitlab.com/acme/x")
	if !errors.Is(err, catalog.ErrRepoUnreadable) {
		t.Errorf("err = %v, want catalog.ErrRepoUnreadable", err)
	}
}

func TestFetchHonoursContextCancellation(t *testing.T) {
	fake := &fakeGitHub{
		repoJSON: `{"default_branch":"main","pushed_at":"2026-01-01T00:00:00Z"}`,
		treeJSON: `{"tree":[]}`,
	}
	srv := fake.server(t)

	f := NewGitHubFetcher("", WithBaseURL(srv.URL), WithClock(testClock))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before we start

	if _, err := f.Fetch(ctx, "github.com/acme/x"); err == nil {
		t.Error("expected an error for a cancelled context")
	}
}
