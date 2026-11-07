// Package push sends Web Push notifications to users' devices.
//
// The package stores subscriptions and its VAPID key pair with the
// [Subscription] and [VAPIDKeys] bun models. The application creates their
// tables in its own migrations, for example in SQLite:
//
//	CREATE TABLE push_subscriptions (
//	  endpoint text PRIMARY KEY,
//	  user_id bigint NOT NULL,
//	  p256dh text NOT NULL,
//	  auth text NOT NULL,
//	  created_at bigint NOT NULL
//	);
//
//	CREATE INDEX push_subscriptions_user_idx ON push_subscriptions(user_id);
//
//	CREATE TABLE vapid_keys (
//	  id integer PRIMARY KEY CHECK (id = 1),
//	  public_key text NOT NULL,
//	  private_key text NOT NULL
//	);
//
// The type of user_id matches the user ID type K of [Service]. The VAPID key
// pair is generated on first start; changing it invalidates all
// subscriptions.
package push

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/uptrace/bun"
)

// Message is a notification shown on the user's device.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is the local path opened when the notification is tapped.
	URL string `json:"url"`
	// Tag makes a newer notification with the same tag replace an older one.
	Tag string `json:"tag,omitempty"`
}

// Notifier notifies users identified by K.
type Notifier[K comparable] interface {
	// Notify sends msg to all devices of users. Delivery happens in the
	// background and failures are only logged.
	Notify(ctx context.Context, userIDs []K, msg Message)
}

// Options configures a [Service].
type Options struct {
	// Subscriber is the contact sent to push services: an email address or
	// an https URL. Apple requires one.
	Subscriber string
	// TTL is how long push services keep undelivered notifications.
	// Defaults to 24 hours.
	TTL time.Duration
	// Client sends the notifications. Defaults to a client with a 30
	// second timeout.
	Client *http.Client
}

// Service stores the push subscriptions of users identified by K and sends
// them notifications with the Web Push protocol.
type Service[K comparable] struct {
	db   bun.IDB
	keys *VAPIDKeys
	opts Options
	wg   sync.WaitGroup
}

var _ Notifier[int64] = (*Service[int64])(nil)

// New creates a push service, generating the VAPID keys on first use.
func New[K comparable](ctx context.Context, db bun.IDB, opts Options) (*Service[K], error) {
	if opts.TTL == 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 30 * time.Second}
	}

	keys, err := getOrCreateKeys(ctx, db, webpush.GenerateVAPIDKeys)
	if err != nil {
		return nil, err
	}

	return &Service[K]{db: db, keys: keys, opts: opts}, nil
}

// PublicKey returns the VAPID public key browsers subscribe with.
func (s *Service[K]) PublicKey() string {
	return s.keys.PublicKey
}

// Notify sends msg to all devices of users in the background.
func (s *Service[K]) Notify(ctx context.Context, userIDs []K, msg Message) {
	if len(userIDs) == 0 {
		return
	}

	subs, err := s.Subscriptions(ctx, userIDs...)
	if err != nil {
		slog.Error("error listing push subscriptions", slog.String("error", err.Error()))
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	for _, sub := range subs {
		// SendNotification appends padding to the message, writing into its
		// spare capacity, so each send needs its own copy.
		s.wg.Go(func() { s.send(sub, slices.Clone(payload)) })
	}
}

// Wait waits for notifications being sent.
func (s *Service[K]) Wait() {
	s.wg.Wait()
}

func (s *Service[K]) send(sub *Subscription[K], payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	log := slog.With(slog.String("push_host", hostOf(sub.Endpoint)), slog.Any("user_id", sub.UserID))

	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		HTTPClient:      s.opts.Client,
		Subscriber:      s.opts.Subscriber,
		VAPIDPublicKey:  s.keys.PublicKey,
		VAPIDPrivateKey: s.keys.PrivateKey,
		TTL:             int(s.opts.TTL.Seconds()),
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		log.Warn("error sending push notification", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	switch {
	case resp.StatusCode < 300:
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The user revoked permission or the subscription expired.
		if err := s.deleteSubscription(ctx, sub.Endpoint); err != nil {
			log.Error("error deleting push subscription", slog.String("error", err.Error()))
		}
	default:
		log.Warn("push service rejected notification",
			slog.Int("status", resp.StatusCode),
			slog.String("body", strings.TrimSpace(string(body))),
		)
	}
}

// pushHosts are the push services the server delivers to. Subscription
// endpoints are supplied by clients, so other hosts are rejected to keep the
// server from making requests anywhere else.
var pushHosts = []string{
	"push.apple.com",            // Safari, iOS
	"fcm.googleapis.com",        // Chrome, Edge on Android
	"push.services.mozilla.com", // Firefox
	"notify.windows.com",        // Edge on Windows
}

// ValidEndpoint reports whether endpoint is an HTTPS URL of a known push
// service.
func ValidEndpoint(endpoint string) bool {
	if len(endpoint) > 1024 {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil {
		return u.Host
	}
	return ""
}
