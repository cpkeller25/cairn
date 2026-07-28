package scorecard

import (
	"testing"
	"time"
)

func TestLevelFor(t *testing.T) {
	tests := []struct {
		name  string
		score int
		want  Level
	}{
		{"zero", 0, LevelBronze},
		{"just below silver", 59, LevelBronze},
		{"exactly silver", 60, LevelSilver},
		{"mid silver", 72, LevelSilver},
		{"just below gold", 84, LevelSilver},
		{"exactly gold", 85, LevelGold},
		{"perfect", 100, LevelGold},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LevelFor(tt.score); got != tt.want {
				t.Errorf("LevelFor(%d) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}

func TestScoreFor(t *testing.T) {
	tests := []struct {
		name          string
		earned, total int
		want          int
	}{
		{"nothing earned", 0, 100, 0},
		{"everything earned", 100, 100, 100},
		{"half", 50, 100, 50},
		{"two thirds rounds up", 2, 3, 67},
		{"one third rounds down", 1, 3, 33},
		{"rouds half away from zero", 1, 8, 13},
		{"zero total is not a divide-by-zero", 0, 0, 0},
		{"negative total is ignored", 5, -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scoreFor(tt.earned, tt.total); got != tt.want {
				t.Errorf("scoreFor(%d, %d) = %d, want %d", tt.earned, tt.total, got, tt.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -3)
	stale := now.AddDate(0, 0, -400)

	perfect := Facts{
		HasReadme:     true,
		HasCI:         true,
		HasTests:      true,
		HasDockerfile: true,
		HasLicense:    true,
		OwnerTeam:     "checkout",
		LastCommitAt:  recent,
		FetchedAt:     now,
	}

	tests := []struct {
		name      string
		facts     Facts
		wantScore int
		wantLevel Level
	}{
		{
			name:      "everything passes",
			facts:     perfect,
			wantScore: 100,
			wantLevel: LevelGold,
		},
		{
			name:      "empty facts fail everything",
			facts:     Facts{FetchedAt: now},
			wantScore: 0,
			wantLevel: LevelBronze,
		},
		{
			name: "missing dockerfile still gold",
			facts: func() Facts {
				f := perfect
				f.HasDockerfile = false
				return f
			}(),
			wantScore: 90,
			wantLevel: LevelGold,
		},
		{
			name: "no tests drops to silver",
			facts: func() Facts {
				f := perfect
				f.HasTests = false
				return f
			}(),
			wantScore: 80,
			wantLevel: LevelSilver,
		},
		{
			name: "stale repo loses recency",
			facts: func() Facts {
				f := perfect
				f.LastCommitAt = stale
				return f
			}(),
			wantScore: 85,
			wantLevel: LevelGold,
		},
		{
			name: "docs only",
			facts: Facts{
				HasReadme:  true,
				HasLicense: true,
				FetchedAt:  now,
			},
			wantScore: 20,
			wantLevel: LevelBronze,
		},
		{
			name: "unowned and untested",
			facts: Facts{
				HasReadme:     true,
				HasCI:         true,
				HasDockerfile: true,
				HasLicense:    true,
				LastCommitAt:  recent,
				FetchedAt:     now,
			},
			wantScore: 65,
			wantLevel: LevelSilver,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.facts)

			if got.OverallScore != tt.wantScore {
				t.Errorf("OverallScore = %d, want %d", got.OverallScore, tt.wantScore)
			}
			if got.Level != tt.wantLevel {
				t.Errorf("Level = %q, want %q", got.Level, tt.wantLevel)
			}
			if len(got.Results) != len(Checks) {
				t.Fatalf("got %d results, want %d", len(got.Results), len(Checks))
			}
		})
	}
}

func TestEvaluateResultsMirrorChecks(t *testing.T) {
	report := Evaluate(Facts{})

	if len(report.Results) != len(Checks) {
		t.Fatalf("got %d results, want %d", len(report.Results), len(Checks))
	}

	for i, r := range report.Results {
		want := Checks[i]
		if r.Key != want.Key {
			t.Errorf("result[%d].Key = %q, want %q", i, r.Key, want.Key)
		}
		if r.Weight != want.Weight {
			t.Errorf("result[%d].Weight = %d, want %d", i, r.Weight, want.Weight)
		}
		if r.Detail == "" {
			t.Errorf("result[%d] (%s) has an empty detail", i, r.Key)
		}
	}
}

func TestWeightsSumTo100(t *testing.T) {
	total := 0
	for _, c := range Checks {
		total += c.Weight
	}
	if total != 100 {
		t.Errorf("weights sum to %d, want 100 "+
			"(scoring still works, but the numbers stop being self-explanatory)", total)
	}
}
