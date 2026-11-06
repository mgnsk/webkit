package web

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"

	"github.com/ggicci/httpin/core"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/wreck"
	sloghttp "github.com/samber/slog-http"
	"maragu.dev/gomponents"
	hxhttp "maragu.dev/gomponents-htmx/http"
)

// ErrorRenderer renders errors recovered by Recover.
type ErrorRenderer struct {
	// Page renders a full error page.
	Page func(r *http.Request, code int, msg string) gomponents.Node
	// Toast renders an error message for htmx requests, swapped into
	// ToastTarget. When nil, htmx requests get the error page.
	Toast func(msg string) gomponents.Node
	// ToastTarget is the CSS selector of the toast area, e.g. "#toast".
	ToastTarget string
}

// ErrorStatus returns the HTTP status code and user facing message for err.
func ErrorStatus(err error) (code int, msg string) {
	code = http.StatusInternalServerError
	msg = "Something went wrong"

	if werr, ok := errors.AsType[*wreck.Error](err); ok {
		// log/slog normalizes int-kind attribute values to int64, so
		// wreck.Value must be read back as int64, not int.
		if v, ok := wreck.Value[int64](werr, webkit.KeyHTTPCode); ok {
			code = int(v)
		}
		msg = cmp.Or(werr.Message(), msg)
	} else if _, ok := errors.AsType[*core.InvalidFieldError](err); ok {
		// httpin.DecodeTo failed to bind a request field.
		code = http.StatusBadRequest
		msg = "Invalid request"
	} else if errors.Is(err, context.DeadlineExceeded) {
		code = http.StatusGatewayTimeout
		msg = "Timeout"
	}

	return code, msg
}

// Recover creates a middleware that recovers panics, logs them and renders
// an error response. Handlers report errors by panicking.
func Recover(renderer ErrorRenderer) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				var err error

				if rec := recover(); rec == nil {
					return
				} else if rec == http.ErrAbortHandler {
					panic(rec)
				} else if rerr, ok := rec.(error); ok {
					err = rerr
				} else {
					err = fmt.Errorf("%v", rec)
				}

				code, msg := ErrorStatus(err)

				if code >= http.StatusInternalServerError {
					stack := make([]byte, 4<<10) // 4 KB
					length := runtime.Stack(stack, false)
					sloghttp.AddCustomAttributes(r, slog.String("stack", string(stack[:length])))
				}
				sloghttp.AddCustomAttributes(r, slog.String("err", err.Error()))

				w.Header().Set("Content-Type", "text/html; charset=UTF-8")

				var node gomponents.Node
				switch {
				case renderer.Toast != nil && hxhttp.IsRequest(r.Header):
					// Let htmx swap the error message into the toast area.
					w.Header().Set("HX-Retarget", renderer.ToastTarget)
					w.Header().Set("HX-Reswap", "innerHTML")
					node = renderer.Toast(msg)
				case renderer.Page != nil:
					node = renderer.Page(r, code, msg)
				default:
					node = gomponents.Textf("Error %d: %s", code, msg)
				}

				w.WriteHeader(code)

				if err := node.Render(w); err != nil {
					slog.Error("error rendering error page", slog.String("error", err.Error()))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
