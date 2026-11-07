package push

import (
	"encoding/json"
	"net/http"

	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/web"
)

// Endpoint paths registered by [Service.Register] and called by the pwa
// package's client script.
const (
	KeyPath         = "/push/key"
	SubscribePath   = "/push/subscribe"
	UnsubscribePath = "/push/unsubscribe"
)

// Register registers the endpoints browsers use to fetch the VAPID public
// key and to subscribe and unsubscribe the current device. userID returns
// the signed-in user of a request, so mws must reject requests without one.
// Errors are reported by panicking, for [web.Recover].
func (s *Service[K]) Register(mux *http.ServeMux, userID func(*http.Request) K, mws ...web.Middleware) {
	mux.Handle("GET "+KeyPath, web.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(s.PublicKey()))
	}), mws...))

	mux.Handle("POST "+SubscribePath, web.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := decodeSubscription(r)
		if sub.Keys.P256dh == "" || sub.Keys.Auth == "" || len(sub.Keys.P256dh) > 256 || len(sub.Keys.Auth) > 256 {
			panic(webkit.InvalidValue.New("Invalid subscription keys"))
		}

		if err := s.Subscribe(r.Context(), &Subscription[K]{
			Endpoint: sub.Endpoint,
			UserID:   userID(r),
			P256dh:   sub.Keys.P256dh,
			Auth:     sub.Keys.Auth,
		}); err != nil {
			panic(err)
		}

		w.WriteHeader(http.StatusNoContent)
	}), mws...))

	mux.Handle("POST "+UnsubscribePath, web.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := decodeSubscription(r)
		if err := s.Unsubscribe(r.Context(), userID(r), sub.Endpoint); err != nil {
			panic(err)
		}

		w.WriteHeader(http.StatusNoContent)
	}), mws...))
}

// subscriptionJSON is a PushSubscription serialized by the browser.
type subscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func decodeSubscription(r *http.Request) subscriptionJSON {
	var s subscriptionJSON
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096)).Decode(&s); err != nil {
		panic(webkit.InvalidValue.New("Invalid subscription", err))
	}
	if !ValidEndpoint(s.Endpoint) {
		panic(webkit.InvalidValue.New("Unsupported push service"))
	}
	return s
}
