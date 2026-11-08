package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	bearerScheme          = "Bearer"
	invalidTokenChallenge = bearerScheme + ` error="invalid_token"`
)

type loggerKey struct{}
type userIDKey struct{}

type Middleware = func(http.Handler) http.Handler

type TokenVerifier interface {
	VerifyToken(tokenStr string) (int64, error)
}

func parseBearer(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], bearerScheme) {
		return "", false
	}
	return parts[1], true
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request, challenge, message string) {
	w.Header().Set("WWW-Authenticate", challenge)
	WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", message)
}

func Auth(verifier TokenVerifier) Middleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			values := r.Header.Values("Authorization")

			switch {
			case len(values) == 0:
				writeUnauthorized(w, r, bearerScheme, "authorization header missing")
				return
			case len(values) > 1:
				writeUnauthorized(w, r, bearerScheme, "multiple authorization headers")
				return
			}

			tokenStr, ok := parseBearer(values[0])
			if !ok {
				writeUnauthorized(w, r, bearerScheme, "invalid authorization header")
				return
			}

			userID, err := verifier.VerifyToken(tokenStr)
			if err != nil {
				writeUnauthorized(w, r, invalidTokenChallenge, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

func UserIDFrom(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey{}).(int64)
	return userID, ok
}

func WithLogger(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			l := log.With("request_id", middleware.GetReqID(r.Context()))

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), loggerKey{}, l)))
		}
		return http.HandlerFunc(fn)
	}
}

func LoggerFrom(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return log
	}

	return slog.Default()
}

func RequestIDHeader() Middleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if id := middleware.GetReqID(r.Context()); id != "" {
				w.Header().Set("X-Request-Id", id)
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

func RequestLogger() Middleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}

				lvl := slog.LevelInfo
				if status >= 500 {
					lvl = slog.LevelError
				}

				LoggerFrom(r.Context()).Log(r.Context(), lvl, "http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"bytes", ww.BytesWritten(),
					"duration", time.Since(start),
					"remote_addr", r.RemoteAddr,
				)
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}

func Recoverer() Middleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				LoggerFrom(r.Context()).Error("panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
				)

				if ww.Status() != 0 {
					panic(http.ErrAbortHandler)
				}

				WriteError(r.Context(), ww, http.StatusInternalServerError, "internal_error", "internal server error")
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}
