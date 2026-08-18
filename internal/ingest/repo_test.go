package ingest

import (
	"errors"
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Repo
	}{
		{"https url", "https://github.com/acme/payments-api", Repo{"acme", "payments-api"}},
		{"no scheme", "github.com/acme/payments-api", Repo{"acme", "payments-api"}},
		{"http scheme", "http://github.com/acme/payments-api", Repo{"acme", "payments-api"}},
		{"trailing slash", "https://github.com/acme/payments-api/", Repo{"acme", "payments-api"}},
		{"got git suffix", "https://github.com/acme/payments-api.git", Repo{"acme", "payments-api"}},
		{"extra path segments", "https://github.com/acme/payments-api/tree/main/src", Repo{"acme", "payments-api"}},
		{"www host", "https://www.github.com/acme/payments-api", Repo{"acme", "payments-api"}},
		{"uppercase host", "https://Github.com/acme/payments-api", Repo{"acme", "payments-api"}},
		{"surrounding whitespace", " https://github.com/acme/payments-api ", Repo{"acme", "payments-api"}},
		{"dots in name", "https://github.com/acme/my.service", Repo{"acme", "my.service"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepoURL(tt.in)
			if err != nil {
				t.Fatalf("ParseRepoURL(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseRepoURLRejects(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"empty", "", ErrInvalidRepoURL},
		{"whitespace only", "   ", ErrInvalidRepoURL},
		{"gitlab", "https://gitlab.com/acme/payments-api", ErrNotGitHub},
		{"self hosted", "https://git.internal.acme.com/acme/svc", ErrNotGitHub},
		{"owner only", "https://github.com/acme", ErrInvalidRepoURL},
		{"host only", "https://github.com", ErrInvalidRepoURL},
		{"host with slash", "https://github.com/", ErrInvalidRepoURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRepoURL(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ParseRepoURL(%q) err = %v, want %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestRepoString(t *testing.T) {
	if got := (Repo{"acme", "payments-api"}).String(); got != "acme/payments-api" {
		t.Errorf("String() = %q, want acme/payments-api", got)
	}
}
