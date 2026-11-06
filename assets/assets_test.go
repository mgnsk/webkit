package assets_test

import (
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/mgnsk/webkit/assets"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "assets")
}

var fsys = fstest.MapFS{
	"app.css":              {Data: []byte("body{}")},
	"sw.js":                {Data: []byte("self")},
	"manifest.webmanifest": {Data: []byte("{}")},
}

func get(a *assets.Assets, target string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	a.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

var _ = Describe("Assets", func() {
	a := assets.New(fsys, assets.WithRootFile("sw.js"))

	It("returns cache-busting paths", func() {
		Expect(a.Path("app.css")).To(Equal(fmt.Sprintf("/assets/app.css?crc=%d", crc32.ChecksumIEEE([]byte("body{}")))))
		Expect(a.Path("app.css")).To(Equal(a.Path("app.css")))
	})

	It("panics on missing assets", func() {
		Expect(func() { a.Path("missing.css") }).To(Panic())
	})

	It("serves assets with immutable caching", func() {
		rec := get(a, a.Path("app.css"))
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("body{}"))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("max-age=2592000, immutable"))
	})

	It("serves web manifests with their mime type", func() {
		rec := get(a, "/assets/manifest.webmanifest")
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/manifest+json"))
	})

	It("serves root files without caching", func() {
		rec := get(a, "/sw.js")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("self"))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("no-cache"))
		Expect(rec.Header().Get("Content-Type")).To(Equal("text/javascript; charset=utf-8"))
	})

	It("does not serve root files that are not registered", func() {
		Expect(get(a, "/app.css").Code).To(Equal(http.StatusNotFound))
	})

	It("is a no-op without a filesystem", func() {
		empty := assets.New(nil)
		Expect(empty.Path("app.css")).To(Equal("app.css"))
		Expect(get(empty, "/assets/app.css").Code).To(Equal(http.StatusNotFound))
	})
})
