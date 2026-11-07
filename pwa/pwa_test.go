package pwa_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/mgnsk/webkit/pwa"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pwa")
}

var opts = pwa.Options{
	Manifest: pwa.Manifest{
		Name:  "app",
		Icons: []pwa.Icon{{Src: "/assets/icon.png", Sizes: "192x192", Type: "image/png"}},
	},
	OfflinePage: "/assets/offline.html",
	Precache:    []string{"/assets/app.css"},
}

func get(p *pwa.PWA, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	p.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// swConfig returns the configuration embedded in the service worker.
func swConfig(p *pwa.PWA) map[string]any {
	GinkgoHelper()
	m := regexp.MustCompile(`(?m)^const config = (.*);$`).FindStringSubmatch(get(p, pwa.ServiceWorkerPath).Body.String())
	Expect(m).To(HaveLen(2))
	var cfg map[string]any
	Expect(json.Unmarshal([]byte(m[1]), &cfg)).To(Succeed())
	return cfg
}

var _ = Describe("PWA", func() {
	It("serves the manifest with defaults", func() {
		p := pwa.New(opts)
		Expect(p.ManifestPath()).To(MatchRegexp(`^/manifest\.webmanifest\?crc=\d+$`))

		rec := get(p, pwa.ManifestPath)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/manifest+json"))
		Expect(rec.Header().Get("Cache-Control")).To(ContainSubstring("immutable"))
		Expect(rec.Body.String()).To(MatchJSON(`{
			"name": "app",
			"id": "/",
			"start_url": "/",
			"scope": "/",
			"display": "standalone",
			"icons": [{"src": "/assets/icon.png", "sizes": "192x192", "type": "image/png"}]
		}`))
	})

	It("serves the service worker from the root without long caching", func() {
		p := pwa.New(opts)
		rec := get(p, "/sw.js")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("no-cache"))
		Expect(rec.Header().Get("Content-Type")).To(HavePrefix("text/javascript"))

		Expect(swConfig(p)).To(MatchKeys(0, Keys{
			"cache":       MatchRegexp(`^pwa-[0-9a-f]{8}$`),
			"title":       Equal("app"),
			"icon":        Equal("/assets/icon.png"),
			"offlinePage": Equal("/assets/offline.html"),
			"precache":    Equal([]any{"/assets/offline.html", "/assets/app.css"}),
		}))
	})

	It("names the cache after the cached files", func() {
		a := swConfig(pwa.New(opts))["cache"]
		b := swConfig(pwa.New(opts))["cache"]
		Expect(a).To(Equal(b))

		changed := opts
		changed.Precache = []string{"/assets/app.css?crc=1"}
		Expect(swConfig(pwa.New(changed))["cache"]).NotTo(Equal(a))
	})

	It("works without an offline page", func() {
		Expect(swConfig(pwa.New(pwa.Options{}))).To(MatchKeys(0, Keys{
			"cache":       Not(BeEmpty()),
			"title":       Equal(""),
			"icon":        Equal(""),
			"offlinePage": Equal(""),
			"precache":    BeEmpty(),
		}))
	})

	It("serves the client script", func() {
		p := pwa.New(opts)
		Expect(p.ScriptPath()).To(MatchRegexp(`^/pwa\.js\?crc=\d+$`))

		rec := get(p, pwa.ScriptPath)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(HavePrefix("text/javascript"))
		Expect(rec.Header().Get("Cache-Control")).To(ContainSubstring("immutable"))
		Expect(rec.Body.String()).To(ContainSubstring("webkit.push"))
	})
})
