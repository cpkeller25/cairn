// Package config loads runtime configuration from the environment
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Config holds everything the service needs to start.
type Config struct {
	// Port is the TCP port to listen on.
	Port string

	// Database URL is a libpq-style connection string, e.g
	// postgres://user:pass@host:5432/dbname?sslmode=disable
	DatabaseURL string

	// GitHubToken authenticates calls to the GitHub API.  Optional: without it,
	// GitHub allows 60 requests/hour by IP rather than 5000/hour by token.
	GitHubToken string

	// LogLevel is one of debug, info, warn, error.
	LogLevel string

	// LogFormat is "json" or "text". JSON for production, text for local reading
	LogFormat string
}

// Load reads configuration from the environment, applying defaults and
// reporting every problem at once.
func Load() (Config, error) {
	cfg := Config{
		Port:        envOr("PORT", "8080"),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		GitHubToken: strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		LogLevel:    strings.ToLower(envOr("LOG_LEVEL", "info")),
		LogFormat:   strings.ToLower(envOr("LOG_FORMAT", "json")),
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s",
			strings.Join(missing, ", "))
	}

	return cfg, nil
}

// Addr returns the listen address for the configured port.
func (c Config) Addr() string {
	return ":" + c.Port
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// SlogLevel maps the configured level onto slog's type, defaulting to Info
func (c Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
