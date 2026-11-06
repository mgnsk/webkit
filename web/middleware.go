// Package web contains HTTP middleware, rendering and request helpers for
// net/http applications using htmx and gomponents.
package web

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/mgnsk/webkit"
	sloghttp "github.com/samber/slog-http"
)

// Middleware is a HTTP middleware function.
type Middleware func(next http.Handler) http.Handler

// Chain applies middlewares to run in order before h.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for _, mw := range slices.Backward(middlewares) {
		h = mw(h)
	}
	return h
}

// Timeout creates a context timeout middleware.
func Timeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CrossOrigin creates a CSRF protection middleware rejecting non-safe
// cross-origin browser requests. Denied requests panic with
// webkit.Forbidden so that Recover renders them.
func CrossOrigin() Middleware {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(webkit.Forbidden.New("Cross-origin request denied"))
	}))
	return cop.Handler
}

// Logger creates a request logging middleware using the default slog
// logger. Server errors and 400-403 client errors are logged.
func Logger() Middleware {
	return sloghttp.NewWithConfig(slog.Default(), sloghttp.Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,

		WithUserAgent: true,
		WithRequestID: true,
		WithClientIP:  true,

		Filters: []sloghttp.Filter{
			func(w sloghttp.WrapResponseWriter, _ *http.Request) bool {
				if w.Status() >= 500 {
					return true
				}

				return w.Status() >= 400 && w.Status() <= 403
			},
		},
	})
}
