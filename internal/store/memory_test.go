package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/scorecard"
)

func TestReportsAreDeepCopied(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	svc := mustService(t, "payments-api")

	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService() error: %v", err)
	}

	report := scorecard.Evaluate(scorecard.Facts{HasReadme: true, FetchedAt: time.Now()})
	if err := s.SaveReport(ctx, svc.ID, report); err != nil {
		t.Fatalf("SaveReport() error: %v", err)
	}

	// Mutating the caller's copy must not affect stored state.
	got, _, _ := s.GetReport(ctx, svc.ID)
	got.Results[0].Passed = !got.Results[0].Passed

	again, _, _ := s.GetReport(ctx, svc.ID)
	if again.Results[0].Passed == got.Results[0].Passed {
		t.Error("mutating a returned report changed the stored report")
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := mustService(t, "svc-"+uuid.New().String()[:8])
			if err := s.CreateService(ctx, svc); err != nil {
				t.Errorf("CreateService() error: %v", err)
				return
			}
			if _, err := s.GetService(ctx, svc.ID); err != nil {
				t.Errorf("GetService() error: %v", err)
			}
			if _, err := s.ListServices(ctx); err != nil {
				t.Errorf("ListServices() error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	got, err := s.ListServices(ctx)
	if err != nil {
		t.Fatalf("ListServices() error: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("got %d services, want 50", len(got))
	}
}
