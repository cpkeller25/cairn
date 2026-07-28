// Package scorecard evaluates repository facts against a weighted set of
// checks, producing a score from 0-100 and a maturity level.
//
// This package performs no I/O.  It has no knowledge of HTTP, Postgres, or
// Github - callers gather Facts and hand them in.
package scorecard

import "time"

// Facts is everything we know about a repository.  It is the sole input to
// the engine
type Facts struct {
	HasReadme     bool
	HasCI         bool
	HasTests      bool
	HasDockerfile bool
	HasLicense    bool

	// OwnerTeam comes from the catalog entry, not from the repo.
	OwnerTeam string

	// LastCommitAt is the timestamp of the most recent commit.
	// The zero value means "unknown".
	LastCommitAt time.Time

	// FetchedAt is when these facts were gathered.  Recency checks measure
	// against this rather than calling time.Now(), which keeps the engine
	// determinisic and testable.
	FetchedAt time.Time
}

// CheckResult is the outcome of running one check against one set of Facts.
type CheckResult struct {
	Key    string
	Passed bool
	Weight int
	Detail string
}

// Level is the maturity tier derived from an overall score.
type Level string

const (
	LevelBronze Level = "bronze"
	LevelSilver Level = "silver"
	LevelGold   Level = "gold"
)

// Report is the complete outcome of evaluating a repository
type Report struct {
	OverallScore int
	Level        Level
	Results      []CheckResult
}
