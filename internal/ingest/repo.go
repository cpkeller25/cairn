package ingest

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	// ErrNotGitHub means the URL points somewhere this adapter cannot read.
	ErrNotGitHub = errors.New("repository is not hosted on github.com")

	// ErrInvalidRepoUrl means the URL does not name an owner and repository.
	ErrInvalidRepoURL = errors.New("repository URL does not name an owner and repository")
)

// Repo identifies a GitHub repository.
type Repo struct {
	Owner string
	Name  string
}

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// ParseRepoURL extracts the owner and repository from a GitHub URL. The
// scheme is optional; a trailing ".git" and any extra path segments are
// ignored.
func ParseRepoURL(raw string) (Repo, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Repo{}, ErrInvalidRepoURL
	}

	// catalog normalises to https://, but accept a bare host too.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Repo{}, fmt.Errorf("%w: %v", ErrInvalidRepoURL, err)
	}

	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return Repo{}, fmt.Errorf("%w: %s", ErrNotGitHub, host)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, ErrInvalidRepoURL
	}

	return Repo{
		Owner: parts[0],
		Name:  strings.TrimSuffix(parts[1], ".git"),
	}, nil
}
