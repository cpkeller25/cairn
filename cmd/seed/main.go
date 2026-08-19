// Command seed registers a handful of well-known public repositories and
// scores them, so a fresh install has something to look at.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cpkeller25/cairn/internal/catalog"
	"github.com/cpkeller25/cairn/internal/config"
	"github.com/cpkeller25/cairn/internal/ingest"
	"github.com/cpkeller25/cairn/internal/scorecard"
	"github.com/cpkeller25/cairn/internal/store"
)

var demoServices = []catalog.NewServiceInput{
	{
		Name:        "go",
		Description: "The Go programming language",
		OwnerTeam:   "language-tools",
		RepoURL:     "https://github.com/golang/go",
		Tier:        1,
	},
	{
		Name:        "kubernets",
		Description: "Production-grade container orchestration",
		OwnerTeam:   "platform",
		RepoURL:     "https://github.com/kubernetes/kubernetes",
		Tier:        1,
	},
	{
		Name:        "prometheus",
		Description: "Monitoring system and time series database",
		OwnerTeam:   "observability",
		RepoURL:     "https://github.com/prometheus/prometheus",
		Tier:        2,
	},
	{
		Name:        "testify",
		Description: "Assertion and mocking toolkit for Go",
		OwnerTeam:   "language-tools",
		RepoURL:     "https://github.com/stretchr/testify",
		Tier:        3,
	},
	{
		Name:        "cairn",
		Description: "This service, scoring itself",
		OwnerTeam:   "platform",
		RepoURL:     "https://github.com/cpkeller25/cairn",
		Tier:        1,
	},
}

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	skipEvaluate := flag.Bool("skip-evaluate", false,
		"register services without calling GitHub")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.Migrate(pool); err != nil {
		return err
	}

	st := store.NewPostgresStore(pool)
	fetcher := ingest.NewGitHubFetcher(cfg.GitHubToken)

	for _, in := range demoServices {
		svc, err := catalog.New(in, time.Now())
		if err != nil {
			return fmt.Errorf("building %s: %w", in.Name, err)
		}

		switch err := st.CreateService(ctx, svc); {
		case errors.Is(err, catalog.ErrNameTaken):
			fmt.Printf("  = %-12s already registered, skipping\n", in.Name)
			continue
		case err != nil:
			return fmt.Errorf("registering %s: %w", in.Name, err)
		}

		if *skipEvaluate {
			fmt.Printf("  + %-12s registered\n", in.Name)
			continue
		}

		facts, err := fetcher.Fetch(ctx, svc.RepoURL)
		if err != nil {
			fmt.Printf("  ! %-12s registered, but could not be evaluated: %v\n", in.Name, err)
			continue
		}
		facts.OwnerTeam = svc.OwnerTeam

		report := scorecard.Evaluate(facts)
		if err := st.SaveReport(ctx, svc.ID, report); err != nil {
			return fmt.Errorf("saving scorecard for %s: %w", in.Name, err)
		}

		fmt.Printf("  + %-12s %3d  %s\n", in.Name, report.OverallScore, report.Level)
	}

	return nil
}
