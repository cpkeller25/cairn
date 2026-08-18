// Package catalog holds the core domain: the Service entity and its rules.
// It performs no I/O.
package catalog

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors describing domain-level failures.  Adapters return these so
// callers can react without knowing which adapter produced them.
var (
	ErrNotFound  = errors.New("service not found")
	ErrNameTaken = errors.New("service name already in use")

	// ErrRepoUnreadable means the service's repository could not be read:
	// wrong host, malformed URL, or the repository does not exist.
	ErrRepoUnreadable = errors.New("repository could not be read")
)

const (
	MinTier     = 1
	MaxTier     = 3
	DefaultTier = 3

	maxNameLen        = 100
	maxOwnerTeamLen   = 100
	maxDescriptionLen = 500
)

// nameFormat allows lowercase alphanumerics separated by single hyphens,
// e.g. "payments-api".  Matches the shape of Kubernets and DNS names.
var nameFormat = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// Service is a registered service in the catalog.
type Service struct {
	ID          uuid.UUID
	Name        string
	Description string
	OwnerTeam   string
	RepoURL     string
	Tier        int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewServiceInput is the caller-supplied data needed to register a service.
// It deliberately excludes ID and timestamps, which the domain assigns
type NewServiceInput struct {
	Name        string
	Description string
	OwnerTeam   string
	RepoURL     string
	Tier        int
}

// New builds a validated Service.  The caller supplies the current time so the
// function stays deterministic and testable.
func New(in NewServiceInput, now time.Time) (Service, error) {
	if in.Tier == 0 {
		in.Tier = DefaultTier
	}

	svc := Service{
		ID:          uuid.New(),
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
		OwnerTeam:   strings.TrimSpace(in.OwnerTeam),
		RepoURL:     normalizeRepoURL(in.RepoURL),
		Tier:        in.Tier,
		// Truncated to microseconds: that is the finest resolution Postgres
		// timestamptz can hold, so keeping nanoseconds would make a
		// store round-trip lossy.
		CreatedAt: now.UTC().Truncate(time.Microsecond),
		UpdatedAt: now.UTC().Truncate(time.Microsecond),
	}

	if err := svc.Validate(); err != nil {
		return Service{}, err
	}
	return svc, nil
}

// Validate reports every rule the service violates, or nil if it is valid.
func (s Service) Validate() error {
	var errs ValidationErrors

	switch {
	case s.Name == "":
		errs = errs.add("name", "is required")
	case len(s.Name) > maxNameLen:
		errs = errs.add("name", fmt.Sprintf("must be at most %d characters", maxNameLen))
	case !nameFormat.MatchString(s.Name):
		errs = errs.add("name", "must be lowercase alphanumeric, optionally hyphen-separated (e.g. payments-api)")
	}

	switch {
	case s.OwnerTeam == "":
		errs = errs.add("owner_team", "is required")
	case len(s.OwnerTeam) > maxOwnerTeamLen:
		errs = errs.add("owner_team", fmt.Sprintf("must be at most %d characters", maxOwnerTeamLen))
	}

	if len(s.Description) > maxDescriptionLen {
		errs = errs.add("description",
			fmt.Sprintf("must be at most %d characters", maxDescriptionLen))
	}

	if s.RepoURL == "" {
		errs = errs.add("repo_url", "is -required")
	} else if !validRepoURL(s.RepoURL) {
		errs = errs.add("repo_url", "must be a valid http(s) URL, e.g. https://github.com/acme/payments-api")
	}

	if s.Tier < MinTier || s.Tier > MaxTier {
		errs = errs.add("tier", fmt.Sprintf("must be between %d and %d", MinTier, MaxTier))
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// normalizeRepoUrl assumes https:// when no scheme is given, so callers may
// write "github.com/acme/payments-api".
func normalizeRepoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return strings.TrimSuffix(raw, "/")
}

func validRepoURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}
