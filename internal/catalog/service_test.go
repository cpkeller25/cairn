package catalog

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validInput() NewServiceInput {
	return NewServiceInput{
		Name:        "payments-api",
		Description: "Handles payment processing",
		OwnerTeam:   "checkout",
		RepoURL:     "https://github.com/acme/payments-api",
		Tier:        1,
	}
}

func TestNewAssignsFields(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

	svc, err := New(validInput(), now)
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}

	if svc.ID.String() == "" {
		t.Error("expected an ID to be assigned")
	}

	if !svc.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", svc.CreatedAt, now)
	}

	if !svc.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", svc.UpdatedAt, now)
	}
	if svc.Name != "payments-api" {
		t.Errorf("Name = %q, want %q", svc.Name, "payments-api")
	}
}

func TestNewGeneratesUniqueIDs(t *testing.T) {
	now := time.Now()

	a, err := New(validInput(), now)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	b, err := New(validInput(), now)
	if err != nil {
		t.Fatalf("new() error: %v", err)
	}

	if a.ID == b.ID {
		t.Error("expected distinct IDs for distinct services")
	}
}

func TestNewDefaultsTier(t *testing.T) {
	in := validInput()
	in.Tier = 0

	svc, err := New(in, time.Now())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if svc.Tier != DefaultTier {
		t.Errorf("Tier = %d, want %d", svc.Tier, DefaultTier)
	}
}

func TestNewTrimsWhitespace(t *testing.T) {
	in := validInput()
	in.Name = "  payments-api  "
	in.OwnerTeam = "\tcheckout\n"

	svc, err := New(in, time.Now())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if svc.Name != "payments-api" {
		t.Errorf("Name = %q, want it trimmed", svc.Name)
	}
	if svc.OwnerTeam != "checkout" {
		t.Errorf("OwnerTeam = %q, want it trimmed", svc.OwnerTeam)
	}
}

func TestNormalizeRepoURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already https", "https://github.com/acme/x", "https://github.com/acme/x"},
		{"scheme added", "github.com/acme/x", "https://github.com/acme/x"},
		{"http preserved", "http://git.internal/acme/x", "http://git.internal/acme/x"},
		{"trailing slash removed", "https://github.com/acme/x/", "https://github.com/acme/x"},
		{"whitespace trimmed", "  github.com/acme/x  ", "https://github.com/acme/x"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRepoURL(tt.in); got != tt.want {
				t.Errorf("normalizeRepoURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*NewServiceInput)
		wantField string
	}{
		{"empty name", func(in *NewServiceInput) { in.Name = "" }, "name"},
		{"name with spaces", func(in *NewServiceInput) { in.Name = "payments api" }, "name"},
		{"name uppercase", func(in *NewServiceInput) { in.Name = "PaymentsAPI" }, "name"},
		{"name leading hyphen", func(in *NewServiceInput) { in.Name = "-payments" }, "name"},
		{"name trailing hyphen", func(in *NewServiceInput) { in.Name = "payments-" }, "name"},
		{"name too long", func(in *NewServiceInput) { in.Name = strings.Repeat("a", 101) }, "name"},

		{"empty owner", func(in *NewServiceInput) { in.OwnerTeam = "" }, "owner_team"},
		{"whitespace owner", func(in *NewServiceInput) { in.OwnerTeam = "   " }, "owner_team"},

		{"empty repo url", func(in *NewServiceInput) { in.RepoURL = "" }, "repo_url"},
		{"repo url no host", func(in *NewServiceInput) { in.RepoURL = "https://" }, "repo_url"},
		{"repo url bad scheme", func(in *NewServiceInput) { in.RepoURL = "ftp://x.com/a" }, "repo_url"},

		{"tier too low", func(in *NewServiceInput) { in.Tier = -1 }, "tier"},
		{"tier too high", func(in *NewServiceInput) { in.Tier = 4 }, "tier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)

			_, err := New(in, time.Now())
			if err == nil {
				t.Fatal("expected a validation error, got nil")
			}

			var verrs ValidationErrors
			if !errors.As(err, &verrs) {
				t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
			}
			found := false
			for _, fe := range verrs {
				if fe.Field == tt.wantField {
					found = true
				}
			}
			if !found {
				t.Errorf("expected an error on field %q, got %v", tt.wantField, verrs)
			}
		})
	}
}

func TestValidateCollectsMultipleErrors(t *testing.T) {
	svc := Service{} // everything invalid at once

	err := svc.Validate()
	if err == nil {
		t.Fatal("expected errors from an empty Service")
	}

	var verrs ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if len(verrs) < 4 {
		t.Errorf("expected at least 4 field errors, got %d: %v", len(verrs), verrs)
	}
}

func TestValidAcceptsEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewServiceInput)
	}{
		{"single character name", func(in *NewServiceInput) { in.Name = "a" }},
		{"numeric name", func(in *NewServiceInput) { in.Name = "svc2" }},
		{"tier 1", func(in *NewServiceInput) { in.Tier = 1 }},
		{"tier 3", func(in *NewServiceInput) { in.Tier = 3 }},
		{"empty description", func(in *NewServiceInput) { in.Description = "" }},
		{"url without scheme", func(in *NewServiceInput) { in.RepoURL = "github.com/acme/x" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)

			if _, err := New(in, time.Now()); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}
