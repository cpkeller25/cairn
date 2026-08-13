package store

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestPostgresStore starts a throwaway Postgres, migrates it, and returns a
// store pointed at it. The container is torn down when the test finishes.
func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping Postgres integration test in -short mode")
	}

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("cairn_test"),
		tcpostgres.WithUsername("cairn"),
		tcpostgres.WithPassword("cairn"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminating container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	pool, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := Migrate(pool); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	return NewPostgresStore(pool)
}

func TestPostgresStoreContract(t *testing.T) {
	runStoreContract(t, func(t *testing.T) storeContract {
		return newTestPostgresStore(t)
	})
}

func TestMigrationsAreIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres integration test in -short mode")
	}

	s := newTestPostgresStore(t)

	// Migrate already ran once inside the helper; running it again must be a
	// no-op rather than an error.
	if err := Migrate(s.pool); err != nil {
		t.Errorf("second Migrate() call failed: %v", err)
	}
}

func TestScorecardHistoryIsRetained(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres integration test in -short mode")
	}

	ctx := context.Background()
	s := newTestPostgresStore(t)

	svc := mustService(t, "payments-api")
	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService() error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := s.SaveReport(ctx, svc.ID, sampleReport()); err != nil {
			t.Fatalf("SaveReport() error: %v", err)
		}
	}

	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM scorecard_results WHERE service_id = $1`, svc.ID).Scan(&count)
	if err != nil {
		t.Fatalf("counting scorecard results: %v", err)
	}
	if count != 3 {
		t.Errorf("got %d scorecard rows, want 3 — history should be retained", count)
	}
}

func TestDeleteCascadesToCheckResults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres integration test in -short mode")
	}

	ctx := context.Background()
	s := newTestPostgresStore(t)

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

	var checks int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM check_results`).Scan(&checks); err != nil {
		t.Fatalf("counting check results: %v", err)
	}
	if checks != 0 {
		t.Errorf("got %d orphaned check_results rows, want 0", checks)
	}
}
