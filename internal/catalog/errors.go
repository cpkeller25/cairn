package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// FieldError is one validation failure on one field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of field failures. It implements error so
// it can be returned anywhere an error is expected, while callers that care
// can type-assert to get the per-field detail.
type ValidationErrors []FieldError

func (v ValidationErrors) add(field, message string) ValidationErrors {
	return append(v, FieldError{Field: field, Message: message})
}

// Error implements the error interface.
func (v ValidationErrors) Error() string {
	if len(v) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(v))
	for _, fe := range v {
		parts = append(parts, fmt.Sprintf("%s %s", fe.Field, fe.Message))
	}
	sort.Strings(parts)
	return "validation failed: " + strings.Join(parts, "; ")
}
