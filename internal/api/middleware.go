package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// ctxKey is unexported so no other package can collide with our context keys.
type ctxKey int

const (
	requestIDKey ctxKey = iota
	loggerKey
)

// RequestIDFrom returns the request ID attached to ctx, or "" if there is none.
func RequestIDForm(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// loggerFrom returns the request-scoped logger, falling back to the default.
func loggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// middleware wraps a handler with additional behaviour.
type middleware func(http.Handler) http.Handler

// chain applies middleware so the first argument is the outermost wrapper.
func chain(h http.Handler, mw ...middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// statusRecorder captures the status code and byte count that a handler wrote,
// which the ResponseWriter interface otherwise does not expose.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK // an implicit 200 from writing without WriteHeader
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// requestID assigns each request an ID, echoes it in the response, and stashes
// a logger pre-tagged with it in the context.  An inbound X-Request-ID is
// honoured so a trace survives across services.
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, id)
		ctx = context.WithValue(ctx, loggerKey, s.logger.With("request_id", id))

		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// logRequests emits one structured line per request once it completes.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health probes and metric scrapes are high-frequency and uninteresting
		switch r.URL.Path {
		case "/metrics", "/healthz", "/readyz":
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		defer func() {
			log := loggerFrom(r.Context())
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			}
			switch {
			case rec.status >= 500:
				log.Error("request failed", attrs...)
			case rec.status >= 400:
				log.Warn("request rejected", attrs...)
			default:
				log.Info("request", attrs...)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

// recoverPanic turns a panic in a handler into a 500 rather than killing the
// process and dropping every other in-flight request.
func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				loggerFrom(r.Context()).Error("panic recovered",
					"panic", rec,
					"path", r.URL.Path,
				)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// recordMetrics observes request counts and latency, labelled by route
// pattern rather than raw path to keep label cardinality bounded.
func (s *Server) recordMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		defer func() {
			// r.Pattern is only populated once ServeMux has matched, so this
			// must be read after the inner handler has run.
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}

			httpRequestsTotal.
				WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).
				Inc()
			httpRequestDuration.
				WithLabelValues(r.Method, route).
				Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(rec, r)
	})
}
