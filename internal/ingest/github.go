package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

const (
	defaultBaseURL   = "https://api.github.com"
	defaultTimeout   = 10 * time.Second
	maxResponseBytes = 10 << 20 // 10 MiB - recursive trees can be large
)

var (
	// ErrRepoNotFound means GitHub retuned 404 for the repository.
	ErrRepoNotFound = errors.New("repository not found on github")

	// ErrRateLimited means the GitHub API rate limit is exhausted.
	ErrRateLimited = errors.New("github api rate limit exceeded")
)

// GitHubFetcher gathers repository facts from the GitHub REST API.
type GitHubFetcher struct {
	client  *http.Client
	baseURL string
	token   string
	now     func() time.Time
}

// Option customises a GitHubFetcher.
type Option func(*GitHubFetcher)

// WithBaseURL points the fetcher at a different API host.  Tests use this to
// aim at an httptest server.
func WithBaseURL(u string) Option {
	return func(f *GitHubFetcher) { f.baseURL = u }
}

// WithHTTPClient supplies a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(f *GitHubFetcher) { f.client = c }
}

// WithClock supplies a fixed clock, so FetchedAt is deterministic in tests.
func WithClock(now func() time.Time) Option {
	return func(f *GitHubFetcher) { f.now = now }
}

// NewGitHubFetcher builds a fetcher.  An empty token means unauthenticated
// requests, which GitHub limits to 60 per hour.
func NewGitHubFetcher(token string, opts ...Option) *GitHubFetcher {
	f := &GitHubFetcher{
		client:  &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBaseURL,
		token:   token,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// get performs an authenticated GET and decodes the JSON body into dst.
func (f *GitHubFetcher) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "cairn")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("calling github: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// carry on
	case http.StatusNotFound:
		return ErrRepoNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return ErrRateLimited
		}
		return fmt.Errorf("github returned %s", resp.Status)
	default:
		return fmt.Errorf("github returned %s", resp.Status)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(dst); err != nil {
		return fmt.Errorf("decoding github response: %w", err)
	}
	return nil
}

// repoResponse is the subset of GET /repos/{owner}/{repo} that we use.
type repoResponse struct {
	FullName      string    `json:"full_name"`
	DefaultBranch string    `json:"default_branch"`
	PushedAt      time.Time `json:"pushed_at"`
	Archived      bool      `json:"archived"`
	License       *struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"license"`
}

// treeResponse is the subset of GET /repos/{owner}/{repo}/git/trees/{sha}.
type treeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"` // "blob" (file) or "tree" (directory)
	} `json:"tree"`
	Truncated bool `json:"truncated"`
}

// Fetch gathers facts about repoURL from the GitHub API.  It makes two
// requests: repository metadata, then the full recursive file tree.
func (f *GitHubFetcher) Fetch(ctx context.Context, repoURL string) (scorecard.Facts, error) {
	repo, err := ParseRepoURL(repoURL)
	if err != nil {
		return scorecard.Facts{}, fmt.Errorf("%w: %v", catalog.ErrRepoUnreadable, err)
	}

	var meta repoResponse
	if err := f.get(ctx, "/repos/"+repo.Owner+"/"+repo.Name, &meta); err != nil {
		if errors.Is(err, ErrRepoNotFound) {
			return scorecard.Facts{}, fmt.Errorf("%w: %s", catalog.ErrRepoUnreadable, repo)
		}
		return scorecard.Facts{}, fmt.Errorf("fetching reepository %s: %w", repo, err)
	}

	branch := meta.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	var tree treeResponse
	treePath := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1",
		repo.Owner, repo.Name, url.PathEscape(branch))
	if err := f.get(ctx, treePath, &tree); err != nil {
		return scorecard.Facts{}, fmt.Errorf("fetching file tree for %s: %w", repo, err)
	}

	paths := make([]string, 0, len(tree.Tree))
	for _, entry := range tree.Tree {
		if entry.Type == "blob" {
			paths = append(paths, entry.Path)
		}
	}

	facts := factsFromPaths(paths)

	// GitHub's own license detection is more reliable than a filename match,
	// but it only recognisses known licences - so accept either signal.
	if meta.License != nil && meta.License.Key != "" {
		facts.HasLicense = true
	}

	facts.LastCommitAt = meta.PushedAt
	facts.FetchedAt = f.now().UTC()

	return facts, nil
}
