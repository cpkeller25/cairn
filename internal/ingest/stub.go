// Package ingest gathers facts about repositories.
package ingest

import (
	"context"
	"hash/fnv"
	"time"

	"github.com/cpkeller25/cairn/internal/scorecard"
)

// StubFetcher produces deterministic pseudo-facts derived from the repository
// URL.  It lets the API be built and demoed before the GitHub adapter exists.
// The same URL always yields the same facts.
type StubFetcher struct {
	Now func() time.Time
}

// New StubFetcher returns a StubFetcher using the real clock.
func NewStubFetcher() *StubFetcher {
	return &StubFetcher{Now: time.Now}
}

// Fetch returns facts for repoURL.  It never fails.
func (f *StubFetcher) Fetch(ctx context.Context, repoURL string) (scorecard.Facts, error) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(repoURL))
	bits := h.Sum32()

	now := f.Now().UTC()

	return scorecard.Facts{
		HasReadme:     bits&1 != 0,
		HasCI:         bits&2 != 0,
		HasTests:      bits&4 != 0,
		HasDockerfile: bits&8 != 0,
		HasLicense:    bits&16 != 0,
		LastCommitAt:  now.AddDate(0, 0, -int(bits%200)),
		FetchedAt:     now,
	}, nil
}
