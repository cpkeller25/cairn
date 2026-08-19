package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/scorecard"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres SQLSTATE codes we translate into domain errors
const pgUniqueViolation = "23505"

// PostgresStore persists the catalog in PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool

	// now is inhjected so tests can pin evaluation timestamps.
	now func() time.Time
}

// Connect opens a connection pool and verifies the database is reachable.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing DATABASE_URL: %w", err)
	}

	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connection to database: %w", err)
	}

	return pool, nil
}

// NewPostgresStore wraps a pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, now: time.Now}
}

// CreateService inserts a service, translating a unique-name violation into
// catalog.ErrNameTaken.
func (s *PostgresStore) CreateService(ctx context.Context, svc catalog.Service) error {
	const q = `
				INSERT INTO services
					(id, name, description, owner_team, repo_url, tier, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := s.pool.Exec(ctx, q, svc.ID, svc.Name, svc.Description, svc.OwnerTeam,
		svc.RepoURL, svc.Tier, svc.CreatedAt, svc.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return catalog.ErrNameTaken
		}
		return fmt.Errorf("inserting service: %w", err)
	}
	return nil
}

// GetService returns one service, or catalog.ErrNotFound
func (s *PostgresStore) GetService(ctx context.Context, id uuid.UUID) (catalog.Service, error) {
	const q = `
				SELECT id, name, description, owner_team, repo_url, tier, created_at, updated_at
				FROM services
				WHERE id = $1`

	var svc catalog.Service
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&svc.ID, &svc.Name, &svc.Description, &svc.OwnerTeam, &svc.RepoURL, &svc.Tier, &svc.CreatedAt, &svc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.Service{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.Service{}, fmt.Errorf("selecting service: %w", err)
	}
	svc.CreatedAt = svc.CreatedAt.UTC()
	svc.UpdatedAt = svc.UpdatedAt.UTC()
	return svc, nil
}

// ListServices returns every service ordered by name.
func (s *PostgresStore) ListServices(ctx context.Context) ([]catalog.Service, error) {
	const q = `
				SELECT id, name, description, owner_team, repo_url, tier, created_at, updated_at
				FROM services
				ORDER BY name`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("selecting services: %w", err)
	}
	defer rows.Close()

	out := make([]catalog.Service, 0)
	for rows.Next() {
		var svc catalog.Service
		if err := rows.Scan(
			&svc.ID, &svc.Name, &svc.Description, &svc.OwnerTeam, &svc.RepoURL, &svc.Tier, &svc.CreatedAt, &svc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning service: %w", err)
		}
		svc.CreatedAt = svc.CreatedAt.UTC()
		svc.UpdatedAt = svc.UpdatedAt.UTC()
		out = append(out, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating services: %w", err)
	}
	return out, nil
}

// DeleteService removes a service. Its scorecards cascade.
func (s *PostgresStore) DeleteService(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM services WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting service: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

// SaveReport writes a scorecard and its check results atomically
func (s *PostgresStore) SaveReport(ctx context.Context, serviceID uuid.UUID, r scorecard.Report) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	// Rollback is a no-op once Commit has succeeded, so this is safe to defer
	// unconditionally: any early return undoes the whole write
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM services WHERE id = $1)`, serviceID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking service exists: %w", err)
	}
	if !exists {
		return catalog.ErrNotFound
	}

	resultID := uuid.New()

	const insertResult = `
		INSERT INTO scorecard_results (id, service_id, overall_score, level, evaluated_at)
		VALUES ($1, $2, $3, $4, $5)`

	if _, err := tx.Exec(ctx, insertResult,
		resultID, serviceID, r.OverallScore, string(r.Level), s.now().UTC(),
	); err != nil {
		return fmt.Errorf("inserting scorecard result: %w", err)
	}

	const insertCheck = `
		INSERT INTO check_results
			(id, result_id, check_key, passed, weight, detail, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	for i, cr := range r.Results {
		if _, err := tx.Exec(ctx, insertCheck,
			uuid.New(), resultID, cr.Key, cr.Passed, cr.Weight, cr.Detail, i,
		); err != nil {
			return fmt.Errorf("inserting check result %q: %w", cr.Key, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing scorecard: %w", err)
	}
	return nil
}

// GetReport returns the most recent scorecard for a service. The boolean
// reports whether one exists; never evaluated is not an error.
func (s *PostgresStore) GetReport(ctx context.Context, serviceID uuid.UUID) (scorecard.Report, bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM services WHERE id = $1)`, serviceID).Scan(&exists)
	if err != nil {
		return scorecard.Report{}, false, fmt.Errorf("checking service exists: %w", err)
	}
	if !exists {
		return scorecard.Report{}, false, catalog.ErrNotFound
	}

	const latest = `
		SELECT id, overall_score, level
		FROM scorecard_results
		WHERE service_id = $1
		ORDER BY evaluated_at DESC
		LIMIT 1`

	var (
		resultID uuid.UUID
		report   scorecard.Report
		level    string
	)
	err = s.pool.QueryRow(ctx, latest, serviceID).Scan(&resultID, &report.OverallScore, &level)
	if errors.Is(err, pgx.ErrNoRows) {
		return scorecard.Report{}, false, nil
	}
	if err != nil {
		return scorecard.Report{}, false, fmt.Errorf("selecting scorecard: %w", err)
	}
	report.Level = scorecard.Level(level)

	const checks = `
		SELECT check_key, passed, weight, detail
		FROM check_results
		WHERE result_id = $1
		ORDER BY position`

	rows, err := s.pool.Query(ctx, checks, resultID)
	if err != nil {
		return scorecard.Report{}, false, fmt.Errorf("selecting check results: %w", err)
	}
	defer rows.Close()

	results := make([]scorecard.CheckResult, 0, len(scorecard.Checks))
	for rows.Next() {
		var cr scorecard.CheckResult
		if err := rows.Scan(&cr.Key, &cr.Passed, &cr.Weight, &cr.Detail); err != nil {
			return scorecard.Report{}, false, fmt.Errorf("scanning check result: %w", err)
		}
		results = append(results, cr)
	}
	if err := rows.Err(); err != nil {
		return scorecard.Report{}, false, fmt.Errorf("iterating check results: %w", err)
	}
	report.Results = results

	return report, true, nil
}
