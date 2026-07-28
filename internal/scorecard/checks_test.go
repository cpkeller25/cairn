package scorecard

import (
	"testing"
	"time"
)

// checkByKey finds a check by its key, failing the test if absent.
func checkByKey(t *testing.T, key string) Check {
	t.Helper()
	for _, c := range Checks {
		if c.Key == key {
			return c
		}
	}
	t.Fatalf("no check registered with key %q", key)
	return Check{}
}

func TestBooleanChecks(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		facts Facts
		want  bool
	}{
		{"readme present", "has_readme", Facts{HasReadme: true}, true},
		{"readme absent", "has_readme", Facts{}, false},

		{"ci present", "has_ci", Facts{HasCI: true}, true},
		{"ci absent", "has_ci", Facts{}, false},

		{"tests present", "has_tests", Facts{HasTests: true}, true},
		{"tests absent", "has_tests", Facts{}, false},

		{"dockerfile present", "has_dockerfile", Facts{HasDockerfile: true}, true},
		{"dockerfile absent", "has_dockerfile", Facts{}, false},

		{"license present", "has_license", Facts{HasLicense: true}, true},
		{"license absent", "has_license", Facts{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := checkByKey(t, tt.key)
			got, detail := c.Eval(tt.facts)
			if got != tt.want {
				t.Errorf("Eval() = %v, want %v (detail: %q)", got, tt.want, detail)
			}
			if detail == "" {
				t.Error("Eval() returned an empty detail string")
			}
		})
	}
}

func TestHasOwner(t *testing.T) {
	tests := []struct {
		name string
		team string
		want bool
	}{
		{"team set", "checkout", true},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"tab and newline only", "\t\n", false},
		{"leading whitespace preserved", "  checkout  ", true},
	}

	c := checkByKey(t, "has_owner")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := c.Eval(Facts{OwnerTeam: tt.team})
			if got != tt.want {
				t.Errorf("Eval(%q) = %v, want %v", tt.team, got, tt.want)
			}
		})
	}
}

func TestRecentActivity(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

	daysAgo := func(d int) time.Time {
		return now.AddDate(0, 0, -d)
	}

	tests := []struct {
		name         string
		lastCommitAt time.Time
		want         bool
	}{
		{"committed today", now, true},
		{"1 day ago", daysAgo(1), true},
		{"89 days ago", daysAgo(89), true},
		{"90 days ago - boundary, inclusive", daysAgo(90), true},
		{"91 days ago - just over", daysAgo(91), false},
		{"a year ago", daysAgo(365), false},
		{"zero value - unknown date", time.Time{}, false},
		{"future commit - clock skew", now.AddDate(0, 0, 5), false},
	}

	c := checkByKey(t, "recent_activity")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := Facts{LastCommitAt: tt.lastCommitAt, FetchedAt: now}
			got, detail := c.Eval(facts)
			if got != tt.want {
				t.Errorf("Eval() = %v, want %v (detail: %q)", got, tt.want, detail)
			}
		})
	}
}

func TestChecksAreWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range Checks {
		if c.Key == "" {
			t.Error("found a check with an empty key")
		}
		if seen[c.Key] {
			t.Errorf("duplicate check key %q", c.Key)
		}
		seen[c.Key] = true

		if c.Weight <= 0 {
			t.Errorf("check %q has non-positive weight %d", c.Key, c.Weight)
		}
		if c.Description == "" {
			t.Errorf("check %q has no description", c.Key)
		}
		if c.Eval == nil {
			t.Errorf("check %q has a nil Eval function", c.Key)
		}
	}
}
