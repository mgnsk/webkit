// Package session provides scs session management, flash messages and
// session user middleware.
package session

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/web"
	hxhttp "maragu.dev/gomponents-htmx/http"
)

// KeyFlashSuccess is the session key of the success flash message.
const KeyFlashSuccess = "flash-success"

// Options configures the session manager.
type Options struct {
	// Secure marks the cookie Secure.
	Secure bool
	// Lifetime is the absolute session lifetime. Defaults to one year.
	Lifetime time.Duration
	// IdleTimeout expires sessions without activity. Zero disables it.
	IdleTimeout time.Duration
}

// New creates a session manager using a persistent, HttpOnly, SameSite=Lax
// cookie named session_id. Session errors panic.
func New(store scs.Store, opts Options) *scs.SessionManager {
	sm := scs.New()
	sm.Store = store
	sm.HashTokenInStore = true
	sm.Lifetime = opts.Lifetime
	if sm.Lifetime == 0 {
		sm.Lifetime = 365 * 24 * time.Hour
	}
	sm.IdleTimeout = opts.IdleTimeout
	sm.Cookie.Name = "session_id"
	sm.Cookie.Domain = ""
	sm.Cookie.HttpOnly = true
	sm.Cookie.Path = "/"
	sm.Cookie.Persist = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = opts.Secure

	sm.ErrorFunc = func(_ http.ResponseWriter, _ *http.Request, err error) {
		panic(err)
	}

	return sm
}

// Middleware creates a middleware that loads and saves the session.
func Middleware(sm *scs.SessionManager) web.Middleware {
	return sm.LoadAndSave
}

// Flash sets a success flash message shown on the next page.
func Flash(ctx context.Context, sm *scs.SessionManager, msg string) {
	sm.Put(ctx, KeyFlashSuccess, msg)
}

// PopFlash returns and removes the success flash message.
// It must be called before writing response headers.
func PopFlash(ctx context.Context, sm *scs.SessionManager) string {
	return sm.PopString(ctx, KeyFlashSuccess)
}

// LoadUser creates a middleware that loads the session user into the
// request context under key. load returns nil when the session has no user.
// When load fails with webkit.NotFound, the user has been deleted: the
// session is destroyed and the client redirected to onMissing.
func LoadUser[U any](
	sm *scs.SessionManager,
	key web.ContextKey[U],
	load func(ctx context.Context, sm *scs.SessionManager) (*U, error),
	onMissing string,
) web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := load(r.Context(), sm)
			if err != nil {
				if !errors.Is(err, webkit.NotFound) {
					panic(err)
				}

				if err := sm.Destroy(r.Context()); err != nil {
					panic(err)
				}
				http.Redirect(w, r, onMissing, http.StatusSeeOther)
				return
			}

			if user != nil {
				r = r.WithContext(key.With(r.Context(), user))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser creates a middleware that requires a user under key.
// Anonymous page requests are redirected to loginPath with a next query
// parameter, other requests fail with webkit.Forbidden.
func RequireUser[U any](key web.ContextKey[U], loginPath string) web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key.Get(r.Context()) == nil {
				if r.Method == http.MethodGet && !hxhttp.IsRequest(r.Header) {
					http.Redirect(w, r, loginPath+"?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
					return
				}
				panic(webkit.Forbidden.New("Must be logged in"))
			}

			next.ServeHTTP(w, r)
		})
	}
}
