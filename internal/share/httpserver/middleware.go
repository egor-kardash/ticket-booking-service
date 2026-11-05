package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type loggerKey struct{}

type Middleware = func(http.Handler) http.Handler

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

			next.ServeHTTP(ww, r)

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
