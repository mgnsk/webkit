package web

import (
	"context"
	"net/http"

	"github.com/ggicci/httpin"
	"maragu.dev/gomponents"
	hxhttp "maragu.dev/gomponents-htmx/http"
)

// Render renders an HTML node. Render errors panic.
func Render(w http.ResponseWriter, node gomponents.Node) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	if err := node.Render(w); err != nil {
		panic(err)
	}
}

// Redirect redirects the client. For htmx requests it sets the HX-Redirect
// header so that htmx performs a full navigation.
func Redirect(w http.ResponseWriter, r *http.Request, location string) {
	if hxhttp.IsRequest(r.Header) {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

// Decode decodes the request into a new V using httpin. Decode errors panic.
func Decode[V any](r *http.Request) *V {
	v := new(V)
	if err := httpin.DecodeTo(r, v); err != nil {
		panic(err)
	}
	return v
}

// ContextKey is a typed context key for values of type *T.
type ContextKey[T any] struct{}

// Get returns the value from ctx or nil.
func (k ContextKey[T]) Get(ctx context.Context) *T {
	v, _ := ctx.Value(k).(*T)
	return v
}

// Lookup returns the value from ctx and whether it was set, which tells a
// value set to nil apart from an unset one.
func (k ContextKey[T]) Lookup(ctx context.Context) (*T, bool) {
	v, ok := ctx.Value(k).(*T)
	return v, ok
}

// With returns a copy of ctx with value v.
func (k ContextKey[T]) With(ctx context.Context, v *T) context.Context {
	return context.WithValue(ctx, k, v)
}
