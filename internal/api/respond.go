package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/cpkeller25/cairn/internal/catalog"
)

// writeJSON serializes v with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status and headers are already sent, so we cannot change the
		// response.  All we can do is record it.
		slog.Default().Error("encoding response", "error", err)
	}
}

// writeError sends a uniform error body.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// writeValidationError sends a 422 with per-field detail.
func writeValidationError(w http.ResponseWriter, verrs catalog.ValidationErrors) {
	writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
		Error:  "validation failed",
		Fields: verrs,
	})
}

// writeDomainError maps a domain or store error onto an HTTP status.
// Unrecognised errors become 500 and are logged, never echoed to the client.
func writeDomainError(ctx context.Context, w http.ResponseWriter, err error) {
	var verrs catalog.ValidationErrors

	switch {
	case errors.As(err, &verrs):
		writeValidationError(w, verrs)
	case errors.Is(err, catalog.ErrNotFound):
		writeError(w, http.StatusNotFound, "service not found")
	case errors.Is(err, catalog.ErrNameTaken):
		writeError(w, http.StatusConflict, "a service with that name already exists")
	default:
		loggerFrom(ctx).Error("unhandled error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
