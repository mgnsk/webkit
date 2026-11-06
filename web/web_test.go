package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/web"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"maragu.dev/gomponents"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "web")
}

var renderer = web.ErrorRenderer{
	Page: func(_ *http.Request, code int, msg string) gomponents.Node {
		return gomponents.Textf("page %d %s", code, msg)
	},
	Toast: func(msg string) gomponents.Node {
		return gomponents.Textf("toast %s", msg)
	},
	ToastTarget: "#toast",
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func panicking(v any) http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(v)
	})
}

type pathInput struct {
	ID int64 `in:"path=id"`
}

var _ = Describe("Recover", func() {
	DescribeTable("maps errors to status codes",
		func(v any, code int, body string) {
			rec := serve(web.Recover(renderer)(panicking(v)), httptest.NewRequest(http.MethodGet, "/", nil))
			Expect(rec.Code).To(Equal(code))
			Expect(rec.Body.String()).To(Equal(body))
			Expect(rec.Header().Get("Content-Type")).To(Equal("text/html; charset=UTF-8"))
		},
		Entry("wreck error", webkit.NotFound.New("Group not found"), 404, "page 404 Group not found"),
		Entry("wrapped wreck error", webkit.Forbidden.New("", errors.New("x")), 403, "page 403 Something went wrong"),
		Entry("deadline", context.DeadlineExceeded, 504, "page 504 Timeout"),
		Entry("plain error", errors.New("boom"), 500, "page 500 Something went wrong"),
		Entry("non-error value", "boom", 500, "page 500 Something went wrong"),
	)

	It("maps httpin decode errors to 400", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /x/{id}", func(_ http.ResponseWriter, r *http.Request) {
			web.Decode[pathInput](r)
		})

		rec := serve(web.Recover(renderer)(mux), httptest.NewRequest(http.MethodGet, "/x/abc", nil))
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(Equal("page 400 Invalid request"))
	})

	It("renders a toast for htmx requests", func() {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("HX-Request", "true")

		rec := serve(web.Recover(renderer)(panicking(webkit.InvalidValue.New("Bad amount"))), req)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Header().Get("HX-Retarget")).To(Equal("#toast"))
		Expect(rec.Header().Get("HX-Reswap")).To(Equal("innerHTML"))
		Expect(rec.Body.String()).To(Equal("toast Bad amount"))
	})

	It("renders the page for htmx requests without a toast renderer", func() {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("HX-Request", "true")

		rec := serve(web.Recover(web.ErrorRenderer{Page: renderer.Page})(panicking(webkit.NotFound.New("Nope"))), req)
		Expect(rec.Header().Get("HX-Retarget")).To(BeEmpty())
		Expect(rec.Body.String()).To(Equal("page 404 Nope"))
	})

	It("re-panics ErrAbortHandler", func() {
		h := web.Recover(renderer)(panicking(http.ErrAbortHandler))
		Expect(func() {
			serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
		}).To(PanicWith(http.ErrAbortHandler))
	})
})

var _ = Describe("NewHandler", func() {
	h := web.NewHandler(web.Options{Errors: renderer, RequestTimeout: time.Second}, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /form", func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			Expect(ok).To(BeTrue())
			Expect(time.Until(deadline)).To(BeNumerically("<=", time.Second))
			w.WriteHeader(http.StatusNoContent)
		})
	})

	It("allows same-origin requests with a timeout", func() {
		req := httptest.NewRequest(http.MethodPost, "/form", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		Expect(serve(h, req).Code).To(Equal(http.StatusNoContent))
	})

	It("rejects cross-origin requests", func() {
		req := httptest.NewRequest(http.MethodPost, "/form", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")

		rec := serve(h, req)
		Expect(rec.Code).To(Equal(http.StatusForbidden))
		Expect(rec.Body.String()).To(Equal("page 403 Cross-origin request denied"))
	})
})

var _ = Describe("Chain", func() {
	It("runs middlewares in order", func() {
		var order []string
		mw := func(name string) web.Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					order = append(order, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		h := web.Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			order = append(order, "handler")
		}), mw("a"), mw("b"))

		serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
		Expect(order).To(Equal([]string{"a", "b", "handler"}))
	})
})

var _ = Describe("Redirect", func() {
	It("uses HX-Redirect for htmx requests", func() {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()

		web.Redirect(rec, req, "/next")
		Expect(rec.Code).To(Equal(http.StatusNoContent))
		Expect(rec.Header().Get("HX-Redirect")).To(Equal("/next"))
	})

	It("uses 303 for plain requests", func() {
		rec := httptest.NewRecorder()

		web.Redirect(rec, httptest.NewRequest(http.MethodPost, "/", nil), "/next")
		Expect(rec.Code).To(Equal(http.StatusSeeOther))
		Expect(rec.Header().Get("Location")).To(Equal("/next"))
	})
})

var _ = Describe("Render", func() {
	It("writes HTML", func() {
		rec := httptest.NewRecorder()
		web.Render(rec, gomponents.El("p", gomponents.Text("<hi>")))
		Expect(rec.Header().Get("Content-Type")).To(Equal("text/html; charset=UTF-8"))
		Expect(rec.Body.String()).To(Equal("<p>&lt;hi&gt;</p>"))
	})
})

var _ = Describe("ContextKey", func() {
	type user struct{ Name string }
	type other struct{ Name string }

	It("stores typed values", func() {
		ctx := web.ContextKey[user]{}.With(context.Background(), &user{Name: "a"})
		Expect(web.ContextKey[user]{}.Get(ctx).Name).To(Equal("a"))
		Expect(web.ContextKey[other]{}.Get(ctx)).To(BeNil())
		Expect(web.ContextKey[user]{}.Get(context.Background())).To(BeNil())
	})
})

var _ = Describe("Decode", func() {
	type form struct {
		Name string `in:"form=name"`
	}

	It("decodes forms", func() {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name=x"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		Expect(web.Decode[form](req).Name).To(Equal("x"))
	})

})
