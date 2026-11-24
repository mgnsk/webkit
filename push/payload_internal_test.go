package push

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestPayload(t *testing.T) {
	msg := Message{Title: "Trip", Body: "alice added Dinner", URL: "/groups/1?tab=log", Tag: "42"}

	for _, tc := range []struct {
		base, want string
	}{
		{"https://example.com", "https://example.com/groups/1?tab=log"},
		{"https://example.com/app/", "https://example.com/groups/1?tab=log"},
		{"", "/groups/1?tab=log"},
	} {
		s := &Service[int64]{}
		if tc.base != "" {
			s.base, _ = url.Parse(tc.base)
		}

		var got map[string]any
		if err := json.Unmarshal(s.payload(msg), &got); err != nil {
			t.Fatal(err)
		}

		want := map[string]any{
			"web_push": float64(8030),
			"notification": map[string]any{
				"title":    "Trip",
				"body":     "alice added Dinner",
				"navigate": tc.want,
				"tag":      "42",
			},
		}
		if b1, b2 := mustJSON(t, got), mustJSON(t, want); b1 != b2 {
			t.Errorf("base %q: got %s, want %s", tc.base, b1, b2)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
