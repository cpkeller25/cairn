package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

func newService(t *testing.T, name string) catalog.Service {
	t.Helper()
	svc, err := catalog.New(catalog.NewServiceInput{
		Name:      name,
		OwnerTeam: "checkout",
		RepoURL:   "https://github.com/acme/" + name,
		Tier:      1,
	}, time.Now())
	if err != nil {
		t.Fatalf("building test service: %v", err)
	}
	return svc
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	svc := newService(t, "payments-api")

	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService() error: %v", err)
	}

	got, err := s.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetService() error: %v", err)
	}
	if got.Name != svc.Name {
		t.Errorf("Name = %q, want %q", got.Name, svc.Name)
	}
}

func TestGetMissingReturnsNotFound(t *testing.T) {
	s := NewMemoryStore()

	_, err := s.GetService(context.Background(), uuid.New())
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("err = %v, want catalog.ErrNotFound", err)
	}
}

func TestCreateRejectsDuplicateName(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	if err := s.CreateService(ctx, newService(t, "payments-api")); err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	err := s.CreateService(ctx, newService(t, "payments-api"))
	if !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("err = %v, want catalog.ErrNameTaken", err)
	}
}

func TestListIsSortedByName(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	for _, name := range []string{"zebra-svc", "alpha-svc", "middle-svc"} {
		if err := s.CreateService(ctx, newService(t, name)); err != nil {
			t.Fatalf("CreateService(%s): %v", name, err)
		}
	}

	got, err := s.ListServices(ctx)
	if err != nil {
		t.Fatalf("ListServices() error: %v", err)
	}

	want := []string{"alpha-svc", "middle-svc", "zebra-svc"}
	if len(got) != len(want) {
		t.Fatalf("got %d services, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("position %d = %q, want %q", i, got[i].Name, want[i])
		}
	}
}

func TestListEmptyReturnsEmptySlice(t *testing.T) {
	got, err := NewMemoryStore().ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices() error: %v", err)
	}
	if got == nil {
		t.Error("got nil slice; want empty non-nil slice so JSON renders [] not null")
	}
	if len(got) != 0 {
		t.Errorf("got %d services, want 0", len(got))
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	svc := newService(t, "payments-api")

	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService() error: %v", err)
	}
	if err := s.DeleteService(ctx, svc.ID); err != nil {
		t.Fatalf("DeleteService() error: %v", err)
	}

	if _, err := s.GetService(ctx, svc.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("after delete, err = %v, want catalog.ErrNotFound", err)
	}
}

func TestDeleteMissingReturnsNotFound(t *testing.T) {
	err := NewMemoryStore().DeleteService(context.Background(), uuid.New())
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("err = %v, want catalog.ErrNotFound", err)
	}
}

func TestReportLifecycle(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	svc := newService(t, "payments-api")

	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService() error: %v", err)
	}

	// Never evaluated: not an error, just absent.
	_, found, err := s.GetReport(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetReport() error: %v", err)
	}
	if found {
		t.Error("found = true for a service that was never evaluated")
	}

	report := scorecard.Evaluate(scorecard.Facts{
		HasReadme: true,
		OwnerTeam: "checkout",
		FetchedAt: time.Now(),
	})
	if err := s.SaveReport(ctx, svc.ID, report); err != nil {
		t.Fatalf("SaveReport() error: %v", err)
	}

	got, found, err := s.GetReport(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetReport() error: %v", err)
	}
	if !found {
		t.Fatal("found = false after SaveReport")
	}
	if got.OverallScore != report.OverallScore {
		t.Errorf("OverallScore = %d, want %d", got.OverallScore, report.OverallScore)
	}
}

func TestSaveReportForMissingService(t *testing.T) {
	err := NewMemoryStore().SaveReport(context.Background(), uuid.New(), scorecard.Report{})
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("err = %v, want catalog.ErrNotFound", err)
	}
}

func TestReportsAreDeepCopied(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	svc := newService(t, "payments-api")

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
			svc := newService(t, "svc-"+uuid.New().String()[:8])
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
