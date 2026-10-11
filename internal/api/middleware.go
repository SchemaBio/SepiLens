package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/config"
)

type ctxKey string

const (
	ctxKeyRequestID ctxKey = "request_id"
	ctxKeyTenantID  ctxKey = "tenant_id"
	ctxKeyPrincipal ctxKey = "principal"
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(ctxKeyRequestID).(string); ok && val != "" {
		return val
	}
	return "req-unknown"
}

func WithTenantID(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantID, tenant)
}

func GetTenantID(ctx context.Context) string {
	if val, ok := ctx.Value(ctxKeyTenantID).(string); ok {
		return val
	}
	return ""
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := ""
		{
			var b [12]byte
			_, _ = rand.Read(b[:])
			reqID = "req-" + hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := WithRequestID(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)

			// Redact query params that might have tokens, don't log bodies or auth headers
			logger.Info("http request",
				"request_id", GetRequestID(r.Context()),
				"tenant_id", GetTenantID(r.Context()),
				"method", r.Method,
				"route", chi.RouteContext(r.Context()).RoutePattern(),
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func RecovererMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "internal server error", false)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/healthz" || path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}

			if cfg.AuthMode == "disabled" {
				ctx := WithTenantID(r.Context(), "anonymous-dev")
				ctx = context.WithValue(ctx, ctxKeyPrincipal, config.Principal{Tenant: "anonymous-dev", Actor: "dev", Role: "admin"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			key := extractAPIKey(r)
			if key == "" {
				WriteError(w, r, http.StatusUnauthorized, ErrCodeAuthFailed, "missing API key in Authorization or X-API-Key header", false)
				return
			}

			principal, ok := cfg.LookupPrincipal(key)
			if !ok {
				WriteError(w, r, http.StatusUnauthorized, ErrCodeAuthFailed, "invalid API key", false)
				return
			}

			ctx := WithTenantID(r.Context(), principal.Tenant)
			ctx = context.WithValue(ctx, ctxKeyPrincipal, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
func principal(r *http.Request) config.Principal {
	p, _ := r.Context().Value(ctxKeyPrincipal).(config.Principal)
	return p
}
func requireRole(w http.ResponseWriter, r *http.Request, roles ...string) bool {
	p := principal(r)
	for _, role := range roles {
		if p.Role == role {
			return true
		}
	}
	WriteError(w, r, http.StatusForbidden, ErrCodeAuthFailed, "role does not allow this operation", false)
	return false
}

func extractAPIKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			return strings.TrimSpace(auth[7:])
		}
		return strings.TrimSpace(auth)
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
