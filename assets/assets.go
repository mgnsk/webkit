// Package assets serves static assets embedded in the binary, with
// checksum query parameters for cache-busting.
package assets

import (
	"fmt"
	"hash/crc32"
	"io/fs"
	"mime"
	"net/http"
	"sync"
	"time"
)

func init() {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		panic(err)
	}
}

// Prefix is the URL path prefix of assets.
const Prefix = "/assets"

// Assets is a static assets filesystem.
type Assets struct {
	fsys    fs.FS
	maxAge  time.Duration
	noCache bool
	paths   sync.Map
}

// Option configures Assets.
type Option func(*Assets)

// WithMaxAge sets the immutable cache max-age of assets. Defaults to 30 days.
func WithMaxAge(d time.Duration) Option {
	return func(a *Assets) { a.maxAge = d }
}

// WithNoCache makes clients revalidate assets on every use instead of
// caching them as immutable.
func WithNoCache() Option {
	return func(a *Assets) { a.noCache = true }
}

// New creates assets from fsys. A nil fsys serves nothing and Path
// returns names unchanged, which is useful in tests.
func New(fsys fs.FS, opts ...Option) *Assets {
	a := &Assets{
		fsys:   fsys,
		maxAge: 30 * 24 * time.Hour,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Path returns the URL path of the asset with an appended CRC checksum
// query parameter for cache-busting. It panics if the asset does not exist.
func (a *Assets) Path(name string) string {
	if a.fsys == nil {
		return name
	}

	if p, ok := a.paths.Load(name); ok {
		return p.(string)
	}

	b, err := fs.ReadFile(a.fsys, name)
	if err != nil {
		panic(err)
	}

	p := fmt.Sprintf("%s/%s?crc=%d", Prefix, name, crc32.ChecksumIEEE(b))
	a.paths.Store(name, p)

	return p
}

// Register registers the assets handlers on mux.
func (a *Assets) Register(mux *http.ServeMux) {
	if a.fsys == nil {
		return
	}

	cacheControl := fmt.Sprintf("max-age=%d, immutable", int64(a.maxAge.Seconds()))
	if a.noCache {
		cacheControl = "no-cache"
	}

	fileServer := http.StripPrefix(Prefix, http.FileServerFS(a.fsys))

	mux.HandleFunc("GET "+Prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheControl)
		fileServer.ServeHTTP(w, r)
	})
}
