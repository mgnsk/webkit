package push

import (
	"context"
	"errors"
	"time"

	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/sqlite"
	"github.com/uptrace/bun"
)

// Subscription is the push subscription of a user's device, stored in the
// push_subscriptions table.
type Subscription[K comparable] struct {
	Endpoint string `bun:"endpoint,pk"`
	UserID   K      `bun:"user_id"`
	P256dh   string `bun:"p256dh"`
	Auth     string `bun:"auth"`
	// CreatedAt is the Unix time in milliseconds when the device last
	// subscribed.
	CreatedAt int64 `bun:"created_at"`

	// The alias is explicit: bun derives the default from the type name,
	// which for a named K such as snowflake.ID contains a package path.
	bun.BaseModel `bun:"push_subscriptions,alias:push_subscriptions"`
}

// VAPIDKeys is the server's VAPID key pair, stored as the only row of the
// vapid_keys table with id 1.
type VAPIDKeys struct {
	ID         int    `bun:"id,pk"`
	PublicKey  string `bun:"public_key"`
	PrivateKey string `bun:"private_key"`

	bun.BaseModel `bun:"vapid_keys"`
}

// Subscribe stores a subscription. A device that subscribes again, possibly
// signed in as another user, replaces its previous subscription.
func (s *Service[K]) Subscribe(ctx context.Context, sub *Subscription[K]) error {
	sub.CreatedAt = time.Now().UnixMilli()
	_, err := s.db.NewInsert().Model(sub).
		On("CONFLICT (endpoint) DO UPDATE").
		Set("user_id = EXCLUDED.user_id").
		Set("p256dh = EXCLUDED.p256dh").
		Set("auth = EXCLUDED.auth").
		Set("created_at = EXCLUDED.created_at").
		Exec(ctx)
	return sqlite.NormalizeError(err)
}

// Unsubscribe deletes a user's subscription by endpoint. Subscriptions of
// other users are left alone.
func (s *Service[K]) Unsubscribe(ctx context.Context, userID K, endpoint string) error {
	_, err := s.db.NewDelete().Model((*Subscription[K])(nil)).
		Where("endpoint = ?", endpoint).
		Where("user_id = ?", userID).
		Exec(ctx)
	return sqlite.NormalizeError(err)
}

// Subscriptions lists the subscriptions of users, oldest first.
func (s *Service[K]) Subscriptions(ctx context.Context, userIDs ...K) ([]*Subscription[K], error) {
	subs := []*Subscription[K]{}
	if len(userIDs) == 0 {
		return subs, nil
	}
	if err := s.db.NewSelect().Model(&subs).
		Where("user_id IN (?)", bun.List(userIDs)).
		Order("created_at ASC").
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return subs, nil
}

// deleteSubscription deletes a subscription the push service no longer
// accepts.
func (s *Service[K]) deleteSubscription(ctx context.Context, endpoint string) error {
	_, err := s.db.NewDelete().Model((*Subscription[K])(nil)).Where("endpoint = ?", endpoint).Exec(ctx)
	return sqlite.NormalizeError(err)
}

// getOrCreateKeys returns the stored VAPID keys, storing the keys returned
// by generate on first use.
func getOrCreateKeys(ctx context.Context, db bun.IDB, generate func() (private, public string, err error)) (*VAPIDKeys, error) {
	keys := &VAPIDKeys{}
	err := db.NewSelect().Model(keys).Where("id = 1").Scan(ctx)
	if err == nil {
		return keys, nil
	}
	if err = sqlite.NormalizeError(err); !errors.Is(err, webkit.NotFound) {
		return nil, err
	}

	private, public, err := generate()
	if err != nil {
		return nil, err
	}

	keys = &VAPIDKeys{ID: 1, PublicKey: public, PrivateKey: private}
	if _, err := db.NewInsert().Model(keys).On("CONFLICT DO NOTHING").Exec(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	// Another process may have won the race; read back what is stored.
	if err := db.NewSelect().Model(keys).Where("id = 1").Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return keys, nil
}
