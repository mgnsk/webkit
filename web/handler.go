package web

import (
	"net/http"
	"time"

	httpin_integration "github.com/ggicci/httpin/integration"
)

func init() {
	httpin_integration.UseHttpPathVariable("path")
}

// Options configures NewHandler.
type Options struct {
	// Errors renders recovered errors.
	Errors ErrorRenderer
	// RequestTimeout is the request context timeout. Defaults to one minute.
	RequestTimeout time.Duration
	// AllowCrossOrigin disables the cross-origin request protection.
	AllowCrossOrigin bool
}

// NewHandler builds an HTTP handler. Each registrar is called with the mux
// to register its routes, then the result is wrapped with the logger,
// recover, cross-origin protection and timeout middleware.
func NewHandler(opts Options, registrars ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()

	for _, register := range registrars {
		register(mux)
	}

	timeout := opts.RequestTimeout
	if timeout == 0 {
		timeout = time.Minute
	}

	mws := []Middleware{
		Logger(),
		Recover(opts.Errors),
	}
	if !opts.AllowCrossOrigin {
		mws = append(mws, CrossOrigin())
	}
	mws = append(mws, Timeout(timeout))

	return Chain(mux, mws...)
}
