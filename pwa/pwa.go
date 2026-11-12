// Package pwa makes a web app installable and able to show push
// notifications: it serves the web app manifest, a service worker and a
// client script.
//
// The service worker shows an offline page for navigations when the network
// is down, displays notifications sent with the push package and opens their
// page when tapped. Data is never cached, so pages are never stale.
//
// The client script registers the service worker, follows notification taps
// in an open window and exposes the push API as window.webkit.push. Include
// it on every page:
//
//	Script(Defer(), Src(app.ScriptPath()))
//
// and link the manifest:
//
//	Link(Rel("manifest"), Href(app.ManifestPath()))
//
// # Client API
//
// window.webkit.pwa.standalone reports whether the page runs as the installed
// app, and window.webkit.pwa.navigating whether it is about to navigate to the
// page of a tapped notification.
//
// window.webkit.push.supported reports whether the browser supports push. The
// installed app on iOS only supports it when opened from the Home Screen.
// The other members return promises:
//
//   - device() resolves to a device once the service worker is ready, or to
//     null without push support. Get it before the user taps a button that
//     subscribes: iOS only shows the permission prompt when
//     device.subscribe() is called directly in the tap handler.
//   - device.subscribed() resolves to whether this device is subscribed.
//   - device.subscribe() asks for permission and subscribes this device for
//     the signed-in user.
//   - device.unsubscribe() unsubscribes this device.
//   - sync() stores this device's subscription for the signed-in user once
//     per browser session and subscription endpoint, e.g. after signing in
//     as another user or when the browser rotated the subscription. The
//     service worker also stores a rotated subscription by itself, while
//     the app is closed.
//   - unsubscribe() unsubscribes this device, giving up after 3 seconds. Call
//     it before signing out.
package pwa

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net/http"
	"strings"
)

// Paths served by [PWA.Register].
const (
	ManifestPath      = "/manifest.webmanifest"
	ServiceWorkerPath = "/sw.js"
	ScriptPath        = "/pwa.js"
)

//go:embed sw.js
var serviceWorkerJS string

//go:embed pwa.js
var scriptJS []byte

// Manifest is a web app manifest.
type Manifest struct {
	Name            string `json:"name"`
	ShortName       string `json:"short_name,omitempty"`
	Description     string `json:"description,omitempty"`
	ID              string `json:"id,omitempty"`
	StartURL        string `json:"start_url"`
	Scope           string `json:"scope,omitempty"`
	Display         string `json:"display,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	ThemeColor      string `json:"theme_color,omitempty"`
	Icons           []Icon `json:"icons,omitempty"`
}

// Icon is a web app manifest icon.
type Icon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes,omitempty"`
	Type    string `json:"type,omitempty"`
	Purpose string `json:"purpose,omitempty"`
}

// Options configures a [PWA].
type Options struct {
	// Manifest is the web app manifest. StartURL, Scope and ID default to
	// "/" and Display to "standalone".
	Manifest Manifest
	// OfflinePage is the URL of the page shown for navigations while
	// offline, e.g. a static asset.
	OfflinePage string
	// Precache are further URLs cached for the offline page, e.g. its
	// stylesheet. Requests for them are answered from the cache while
	// offline.
	Precache []string
	// Icon is the notification icon URL. Defaults to the first manifest
	// icon.
	Icon string
}

// PWA serves the manifest, service worker and client script of an app.
type PWA struct {
	manifest      []byte
	serviceWorker []byte
}

// New creates a PWA.
func New(opts Options) *PWA {
	m := opts.Manifest
	if m.StartURL == "" {
		m.StartURL = "/"
	}
	if m.Scope == "" {
		m.Scope = "/"
	}
	if m.ID == "" {
		m.ID = "/"
	}
	if m.Display == "" {
		m.Display = "standalone"
	}
	if opts.Icon == "" && len(m.Icons) > 0 {
		opts.Icon = m.Icons[0].Src
	}

	manifest, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}

	cfg := struct {
		Cache       string   `json:"cache"`
		Title       string   `json:"title"`
		Icon        string   `json:"icon"`
		OfflinePage string   `json:"offlinePage"`
		Precache    []string `json:"precache"`
	}{
		Title:       m.Name,
		Icon:        opts.Icon,
		OfflinePage: opts.OfflinePage,
		Precache:    []string{},
	}
	if opts.OfflinePage != "" {
		cfg.Precache = append(cfg.Precache, opts.OfflinePage)
	}
	cfg.Precache = append(cfg.Precache, opts.Precache...)

	// Name the cache after its contents so that a change replaces it.
	b, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	cfg.Cache = fmt.Sprintf("pwa-%08x", crc32.ChecksumIEEE(b))

	b, err = json.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	return &PWA{
		manifest:      manifest,
		serviceWorker: []byte(strings.Replace(serviceWorkerJS, "/*CONFIG*/{}", string(b), 1)),
	}
}

// ManifestPath returns the URL path of the manifest with a checksum query
// parameter for cache-busting.
func (p *PWA) ManifestPath() string {
	return fmt.Sprintf("%s?crc=%d", ManifestPath, crc32.ChecksumIEEE(p.manifest))
}

// ScriptPath returns the URL path of the client script with a checksum
// query parameter for cache-busting.
func (*PWA) ScriptPath() string {
	return fmt.Sprintf("%s?crc=%d", ScriptPath, crc32.ChecksumIEEE(scriptJS))
}

// Register registers the manifest, service worker and client script
// handlers on mux. The service worker is served from the root to control the
// whole app, and is not cached so that updates are picked up.
func (p *PWA) Register(mux *http.ServeMux) {
	immutable := "max-age=2592000, immutable"

	mux.HandleFunc("GET "+ManifestPath, serve(p.manifest, "application/manifest+json", immutable))
	mux.HandleFunc("GET "+ServiceWorkerPath, serve(p.serviceWorker, "text/javascript; charset=utf-8", "no-cache"))
	mux.HandleFunc("GET "+ScriptPath, serve(scriptJS, "text/javascript; charset=utf-8", immutable))
}

func serve(body []byte, contentType, cacheControl string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", cacheControl)
		_, _ = w.Write(body)
	}
}
