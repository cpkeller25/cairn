package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
)

// storeContract is the behaviour every store implementation must provide
// It mirros api.Store; declaring it here keeps this package free of an
// import on api.
type storeContract interface {
	CreateService(ctx context.Context, svc catalog.Service) error
	GetService(ctx context.Context, id uuid.UUID) (catalog.Service, error)
	ListServices(ctx context.Context) ([]catalog.Service, error)
	DeleteService(ctx context.Context, id uuid.UUID) error
	SaveReport(ctx context.Context, serviceID uuid.UUID, r scorecard.Report) error
	GetReport(ctx context.Context, serviceID uuid.UUID) (scorecard.Report, bool, error)
}

func mustService(t *testing.T, name string) catalog.Service {
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

func sampleReport() scorecard.Report {
	return scorecard.Evaluate(scorecard.Facts{
		HasReadme:    true,
		HasCI:        true,
		OwnerTeam:    "checkout",
		LastCommitAt: time.Now().AddDate(0, 0, -3),
		FetchedAt:    time.Now(),
	})
}

// runStoreContract exercises the behaviour every store must share.
// newStore must return a fresh, empty store on each call.
func runStoreContract(t *testing.T, newStore func(t *testing.T) storeContract) {
	ctx := context.Background()

	t.Run("create then get", func(t *testing.T) {
		s := newStore(t)
		svc := mustService(t, "payments-api")

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

		if got.OwnerTeam != svc.OwnerTeam {
			t.Errorf("OwnerTeam = %q, want %q", got.OwnerTeam, svc.OwnerTeam)
		}
		if got.Tier != svc.Tier {
			t.Errorf("Tier = %d, want %d", got.Tier, svc.Tier)
		}
		if !got.CreatedAt.Equal(svc.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, svc.CreatedAt)
		}
	})

	t.Run("get missing is ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.GetService(ctx, uuid.New()); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("err = %v, want catalog.ErrNotFound", err)
		}
	})

	t.Run("duplicate name is ErrNameTaken", func(t *testing.T) {
		s := newStore(t)
		if err := s.CreateService(ctx, mustService(t, "payments-api")); err != nil {
			t.Fatalf("first create: %v", err)
		}
		err := s.CreateService(ctx, mustService(t, "payments-api"))
		if !errors.Is(err, catalog.ErrNameTaken) {
			t.Errorf("err = %v, want catalog.ErrNameTaken", err)
		}
	})

	t.Run("list is sorted by name", func(t *testing.T) {
		s := newStore(t)
		for _, n := range []string{"zebra-svc", "alpha-svc", "middle-svc"} {
			if err := s.CreateService(ctx, mustService(t, n)); err != nil {
				t.Fatalf("CreateService(%s): %v", n, err)
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
	})

	t.Run("empty list is non-nil", func(t *testing.T) {
		s := newStore(t)
		got, err := s.ListServices(ctx)
		if err != nil {
			t.Fatalf("ListServices() error: %v", err)
		}
		if got == nil {
			t.Error("got nil slice; want empty non-nil so JSON renders [] not null")
		}
	})

	t.Run("delete removes the service", func(t *testing.T) {
		s := newStore(t)
		svc := mustService(t, "payments-api")
		if err := s.CreateService(ctx, svc); err != nil {
			t.Fatalf("CreateService() error: %v", err)
		}
		if err := s.DeleteService(ctx, svc.ID); err != nil {
			t.Fatalf("DeleteService() error: %v", err)
		}
		if _, err := s.GetService(ctx, svc.ID); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("after delete, err = %v, want catalog.ErrNotFound", err)
		}
	})

	t.Run("delete missing is ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if err := s.DeleteService(ctx, uuid.New()); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("err = %v, want catalog.ErrNotFound", err)
		}
	})

	t.Run("unevaluated service has no report", func(t *testing.T) {
		s := newStore(t)
		svc := mustService(t, "payments-api")
		if err := s.CreateService(ctx, svc); err != nil {
			t.Fatalf("CreateService() error: %v", err)
		}

		_, found, err := s.GetReport(ctx, svc.ID)
		if err != nil {
			t.Fatalf("GetReport() error: %v", err)
		}
		if found {
			t.Error("found = true for a service that was never evaluated")
		}
	})

	t.Run("save then get report", func(t *testing.T) {
		s := newStore(t)
		svc := mustService(t, "payments-api")
		if err := s.CreateService(ctx, svc); err != nil {
			t.Fatalf("CreateService() error: %v", err)
		}

		want := sampleReport()
		if err := s.SaveReport(ctx, svc.ID, want); err != nil {
			t.Fatalf("SaveReport() error: %v", err)
		}

		got, found, err := s.GetReport(ctx, svc.ID)
		if err != nil {
			t.Fatalf("GetReport() error: %v", err)
		}
		if !found {
			t.Fatal("found = false after SaveReport")
		}
		if got.OverallScore != want.OverallScore {
			t.Errorf("OverallScore = %d, want %d", got.OverallScore, want.OverallScore)
		}
		if got.Level != want.Level {
			t.Errorf("Level = %q, want %q", got.Level, want.Level)
		}
		if len(got.Results) != len(want.Results) {
			t.Fatalf("got %d check results, want %d", len(got.Results), len(want.Results))
		}
		// Order must be preserved.
		for i := range want.Results {
			if got.Results[i].Key != want.Results[i].Key {
				t.Errorf("result %d key = %q, want %q", i, got.Results[i].Key, want.Results[i].Key)
			}
			if got.Results[i].Passed != want.Results[i].Passed {
				t.Errorf("result %q passed = %v, want %v",
					got.Results[i].Key, got.Results[i].Passed, want.Results[i].Passed)
			}
			if got.Results[i].Weight != want.Results[i].Weight {
				t.Errorf("result %q weight = %d, want %d",
					got.Results[i].Key, got.Results[i].Weight, want.Results[i].Weight)
			}
		}
	})

	t.Run("re-evaluating returns the latest report", func(t *testing.T) {
		s := newStore(t)
		svc := mustService(t, "payments-api")
		if err := s.CreateService(ctx, svc); err != nil {
			t.Fatalf("CreateService() error: %v", err)
		}

		first := scorecard.Evaluate(scorecard.Facts{HasReadme: true, FetchedAt: time.Now()})
		if err := s.SaveReport(ctx, svc.ID, first); err != nil {
			t.Fatalf("SaveReport() error: %v", err)
		}

		second := sampleReport()
		if err := s.SaveReport(ctx, svc.ID, second); err != nil {
			t.Fatalf("SaveReport() error: %v", err)
		}

		got, found, err := s.GetReport(ctx, svc.ID)
		if err != nil || !found {
			t.Fatalf("GetReport() = %v, found=%v", err, found)
		}
		if got.OverallScore != second.OverallScore {
			t.Errorf("OverallScore = %d, want the most recent (%d)",
				got.OverallScore, second.OverallScore)
		}
	})

	t.Run("report for missing service is ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if err := s.SaveReport(ctx, uuid.New(), sampleReport()); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("SaveReport err = %v, want catalog.ErrNotFound", err)
		}
		if _, _, err := s.GetReport(ctx, uuid.New()); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("GetReport err = %v, want catalog.ErrNotFound", err)
		}
	})

	t.Run("deleting a service removes its report", func(t *testing.T) {
		s := newStore(t)
		svc := mustService(t, "payments-api")
		if err := s.CreateService(ctx, svc); err != nil {
			t.Fatalf("CreateService() error: %v", err)
		}
		if err := s.SaveReport(ctx, svc.ID, sampleReport()); err != nil {
			t.Fatalf("SaveReport() error: %v", err)
		}
		if err := s.DeleteService(ctx, svc.ID); err != nil {
			t.Fatalf("DeleteService() error: %v", err)
		}
		if _, _, err := s.GetReport(ctx, svc.ID); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("err = %v, want catalog.ErrNotFound", err)
		}
	})
}

func TestMemoryStoreContract(t *testing.T) {
	runStoreContract(t, func(t *testing.T) storeContract {
		return NewMemoryStore()
	})
}
