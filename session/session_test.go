package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/session"
	"github.com/mgnsk/webkit/web"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "session")
}

type user struct{ ID int64 }

var key web.ContextKey[user]

var _ = Describe("session", func() {
	var (
		sm    *scs.SessionManager
		users map[int64]*user
		h     http.Handler
	)

	BeforeEach(func() {
		sm = session.New(memstore.New(), session.Options{})
		users = map[int64]*user{1: {ID: 1}}

		mux := http.NewServeMux()
		mux.HandleFunc("GET /login/{id}", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "user_id", int64(1))
			session.Flash(r.Context(), sm, "Signed in")
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("/private", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(session.PopFlash(r.Context(), sm)))
		})

		h = web.Chain(mux,
			web.Recover(web.ErrorRenderer{}),
			session.Middleware(sm),
			session.LoadUser(sm, key, func(ctx context.Context, sm *scs.SessionManager) (*user, error) {
				id := sm.GetInt64(ctx, "user_id")
				if id == 0 {
					return nil, nil
				}
				if u, ok := users[id]; ok {
					return u, nil
				}
				return nil, webkit.NotFound.New("User not found")
			}, "/login"),
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/private" {
						session.RequireUser(key, "/login")(next).ServeHTTP(w, r)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		)
	})

	do := func(method, target string, cookies []*http.Cookie, htmx bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	It("configures the cookie", func() {
		Expect(sm.Cookie.Name).To(Equal("session_id"))
		Expect(sm.Cookie.HttpOnly).To(BeTrue())
		Expect(sm.Cookie.SameSite).To(Equal(http.SameSiteLaxMode))
		Expect(sm.Lifetime).NotTo(BeZero())
	})

	It("redirects anonymous page requests to login", func() {
		rec := do(http.MethodGet, "/private?a=b", nil, false)
		Expect(rec.Code).To(Equal(http.StatusSeeOther))
		Expect(rec.Header().Get("Location")).To(Equal("/login?next=%2Fprivate%3Fa%3Db"))
	})

	It("forbids anonymous htmx and non-GET requests", func() {
		Expect(do(http.MethodGet, "/private", nil, true).Code).To(Equal(http.StatusForbidden))
		Expect(do(http.MethodPost, "/private", nil, false).Code).To(Equal(http.StatusForbidden))
	})

	It("loads the user and pops the flash once", func() {
		cookies := do(http.MethodGet, "/login/1", nil, false).Result().Cookies()
		Expect(cookies).To(HaveLen(1))

		rec := do(http.MethodGet, "/private", cookies, false)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("Signed in"))

		rec = do(http.MethodGet, "/private", cookies, false)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(BeEmpty())
	})

	It("destroys the session of a deleted user", func() {
		cookies := do(http.MethodGet, "/login/1", nil, false).Result().Cookies()
		delete(users, 1)

		rec := do(http.MethodGet, "/private", cookies, false)
		Expect(rec.Code).To(Equal(http.StatusSeeOther))
		Expect(rec.Header().Get("Location")).To(Equal("/login"))

		users[1] = &user{ID: 1}
		rec = do(http.MethodGet, "/private", cookies, false)
		Expect(rec.Code).To(Equal(http.StatusSeeOther))
		Expect(rec.Header().Get("Location")).To(HavePrefix("/login?next="))
	})
})
