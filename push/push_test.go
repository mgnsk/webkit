package push_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mgnsk/webkit/push"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/sqlite"
	. "github.com/mgnsk/webkit/testutil"
	"github.com/mgnsk/webkit/web"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"
	"maragu.dev/gomponents"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "push")
}

// schema is the example schema from the package documentation.
const schema = `
CREATE TABLE push_subscriptions (
  endpoint text PRIMARY KEY,
  user_id bigint NOT NULL,
  p256dh text NOT NULL,
  auth text NOT NULL,
  created_at bigint NOT NULL
);

CREATE INDEX push_subscriptions_user_idx ON push_subscriptions(user_id);

CREATE TABLE vapid_keys (
  id integer PRIMARY KEY CHECK (id = 1),
  public_key text NOT NULL,
  private_key text NOT NULL
);
`

var db *bun.DB

// namedID is a named user ID type from another package, like
// snowflake.ID.
type namedID = snowflake.ID

var _ = BeforeEach(func() {
	db = sqlite.NewDB(":memory:").Connect()
	DeferCleanup(db.Close)

	_, err := db.Exec(schema)
	Expect(err).NotTo(HaveOccurred())
})

// newSubscription returns a subscription with valid encryption keys.
func newSubscription(userID int64, endpoint string) *push.Subscription[int64] {
	GinkgoHelper()
	key := Must(ecdh.P256().GenerateKey(rand.Reader))
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return &push.Subscription[int64]{
		Endpoint: endpoint,
		UserID:   userID,
		P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
}

var _ = Describe("ValidEndpoint", func() {
	DescribeTable("accepts known push services only",
		func(endpoint string, ok bool) {
			Expect(push.ValidEndpoint(endpoint)).To(Equal(ok))
		},
		Entry(nil, "https://web.push.apple.com/abc", true),
		Entry(nil, "https://fcm.googleapis.com/fcm/send/abc", true),
		Entry(nil, "https://updates.push.services.mozilla.com/wpush/v2/abc", true),
		Entry(nil, "https://wns2-par02p.notify.windows.com/w/?token=abc", true),
		Entry(nil, "http://web.push.apple.com/abc", false),
		Entry(nil, "https://web.push.apple.com:8443/abc", false),
		Entry(nil, "https://user@web.push.apple.com/abc", false),
		Entry(nil, "https://push.apple.com.evil.test/abc", false),
		Entry(nil, "https://evilpush.apple.com.test/abc", false),
		Entry(nil, "https://localhost/abc", false),
		Entry(nil, "https://"+strings.Repeat("a", 1100)+".push.apple.com/", false),
	)
})

var _ = Describe("Service", func() {
	It("keeps its VAPID keys", func(ctx SpecContext) {
		a := Must(push.New[int64](ctx, db, push.Options{}))
		b := Must(push.New[int64](ctx, db, push.Options{}))
		Expect(a.PublicKey()).NotTo(BeEmpty())
		Expect(b.PublicKey()).To(Equal(a.PublicKey()))
	})

	It("stores, moves and deletes subscriptions", func(ctx SpecContext) {
		s := Must(push.New[int64](ctx, db, push.Options{}))

		Expect(s.Subscribe(ctx, newSubscription(1, "https://web.push.apple.com/a"))).To(Succeed())
		Expect(s.Subscribe(ctx, newSubscription(1, "https://web.push.apple.com/b"))).To(Succeed())
		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveLen(2))

		// Another user can not remove the subscription.
		Expect(s.Unsubscribe(ctx, 2, "https://web.push.apple.com/a")).To(Succeed())
		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveLen(2))

		// The same device subscribing as another user moves it.
		Expect(s.Subscribe(ctx, newSubscription(2, "https://web.push.apple.com/a"))).To(Succeed())
		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveExactElements(HaveField("Endpoint", "https://web.push.apple.com/b")))
		Expect(Must(s.Subscriptions(ctx, 2))).To(HaveExactElements(HaveField("Endpoint", "https://web.push.apple.com/a")))
		Expect(Must(s.Subscriptions(ctx, 1, 2, 2))).To(HaveLen(2))

		Expect(s.Unsubscribe(ctx, 2, "https://web.push.apple.com/a")).To(Succeed())
		Expect(Must(s.Subscriptions(ctx, 2))).To(BeEmpty())
		Expect(Must(s.Subscriptions(ctx))).To(BeEmpty())
	})

	It("supports named user ID types", func(ctx SpecContext) {
		s := Must(push.New[namedID](ctx, db, push.Options{}))
		Expect(s.Subscribe(ctx, &push.Subscription[namedID]{Endpoint: "https://web.push.apple.com/a", UserID: 1})).To(Succeed())
		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveLen(1))
		Expect(s.Unsubscribe(ctx, 1, "https://web.push.apple.com/a")).To(Succeed())
		Expect(Must(s.Subscriptions(ctx, 1))).To(BeEmpty())
	})

	It("sends encrypted notifications and drops expired subscriptions", func(ctx SpecContext) {
		var (
			mu     sync.Mutex
			reqs   []*http.Request
			bodies []int
		)
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			reqs = append(reqs, r)
			bodies = append(bodies, len(b))
			mu.Unlock()
			if r.URL.Path == "/gone" {
				w.WriteHeader(http.StatusGone)
				return
			}
			w.WriteHeader(http.StatusCreated)
		}))
		DeferCleanup(ts.Close)

		s := Must(push.New[int64](ctx, db, push.Options{
			Subscriber: "admin@example.com",
			Client:     ts.Client(),
		}))
		Expect(s.Subscribe(ctx, newSubscription(1, ts.URL+"/ok"))).To(Succeed())
		Expect(s.Subscribe(ctx, newSubscription(1, ts.URL+"/gone"))).To(Succeed())
		Expect(s.Subscribe(ctx, newSubscription(2, ts.URL+"/other"))).To(Succeed())

		s.Notify(ctx, []int64{1}, push.Message{Title: "Trip", Body: "alice added Dinner", URL: "/"})
		s.Wait()

		Expect(reqs).To(HaveLen(2))
		for i, r := range reqs {
			Expect(r.URL.Path).NotTo(Equal("/other"))
			Expect(r.Header.Get("Content-Encoding")).To(Equal("aes128gcm"))
			Expect(r.Header.Get("TTL")).To(Equal("86400"))
			Expect(r.Header.Get("Authorization")).To(HavePrefix("vapid t="))
			Expect(r.Header.Get("Authorization")).To(ContainSubstring(", k=" + s.PublicKey()))
			Expect(bodies[i]).To(BeNumerically(">", 0))
		}

		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveExactElements(HaveField("Endpoint", ts.URL+"/ok")))
	})
})

var _ = Describe("Register", func() {
	const endpoint = "https://web.push.apple.com/QGx3abc"

	var (
		s *push.Service[int64]
		h http.Handler
	)

	BeforeEach(func(ctx SpecContext) {
		s = Must(push.New[int64](ctx, db, push.Options{}))

		// The test user is the X-User header, missing for signed out requests.
		requireUser := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-User") == "" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		userID := func(r *http.Request) int64 {
			return int64(len(r.Header.Get("X-User")))
		}

		h = web.NewHandler(web.Options{Errors: web.ErrorRenderer{
			Page: func(_ *http.Request, code int, msg string) gomponents.Node {
				return gomponents.Textf("%d %s", code, msg)
			},
		}}, func(mux *http.ServeMux) {
			s.Register(mux, userID, requireUser)
		})
	})

	do := func(method, path, user, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if user != "" {
			req.Header.Set("X-User", user)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	body := func(endpoint string) string {
		return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"BNcR","auth":"tBHI"}}`
	}

	It("serves the VAPID key", func() {
		rec := do(http.MethodGet, push.KeyPath, "a", "")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal(s.PublicKey()))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("no-store"))
	})

	It("subscribes and unsubscribes the signed-in user", func(ctx SpecContext) {
		Expect(do(http.MethodPost, push.SubscribePath, "a", body(endpoint)).Code).To(Equal(http.StatusNoContent))
		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveExactElements(And(
			HaveField("Endpoint", endpoint),
			HaveField("P256dh", "BNcR"),
			HaveField("Auth", "tBHI"),
		)))

		// Another user can not remove the subscription.
		Expect(do(http.MethodPost, push.UnsubscribePath, "bb", body(endpoint)).Code).To(Equal(http.StatusNoContent))
		Expect(Must(s.Subscriptions(ctx, 1))).To(HaveLen(1))

		Expect(do(http.MethodPost, push.UnsubscribePath, "a", body(endpoint)).Code).To(Equal(http.StatusNoContent))
		Expect(Must(s.Subscriptions(ctx, 1))).To(BeEmpty())
	})

	It("rejects invalid subscriptions", func(ctx SpecContext) {
		for _, b := range []string{
			body("https://example.com/push"),
			body("http://web.push.apple.com/x"),
			body("https://push.apple.com.evil.test/x"),
			body("https://127.0.0.1/x"),
			`{"endpoint":"` + endpoint + `"}`,
			`not json`,
		} {
			rec := do(http.MethodPost, push.SubscribePath, "a", b)
			Expect(rec.Code).To(Equal(http.StatusBadRequest), b)
		}
		Expect(Must(s.Subscriptions(ctx, 1))).To(BeEmpty())
	})

	It("applies the middleware", func() {
		Expect(do(http.MethodGet, push.KeyPath, "", "").Code).To(Equal(http.StatusForbidden))
		Expect(do(http.MethodPost, push.SubscribePath, "", body(endpoint)).Code).To(Equal(http.StatusForbidden))
		Expect(do(http.MethodPost, push.UnsubscribePath, "", body(endpoint)).Code).To(Equal(http.StatusForbidden))
	})
})
