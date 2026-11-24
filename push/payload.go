package push

import (
	"encoding/json"
	"net/url"
)

// declarativeWebPush is the value of the web_push member that marks a
// Declarative Web Push message.
const declarativeWebPush = 8030

// declarativeMessage is a Declarative Web Push message.
type declarativeMessage struct {
	WebPush      int                     `json:"web_push"`
	Notification declarativeNotification `json:"notification"`
}

type declarativeNotification struct {
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	Navigate string `json:"navigate"`
	Tag      string `json:"tag,omitempty"`
}

// payload encodes msg as a Declarative Web Push message, with the URL
// resolved against the base URL.
func (s *Service[K]) payload(msg Message) []byte {
	navigate := msg.URL
	if navigate == "" {
		navigate = "/"
	}
	if s.base != nil {
		if u, err := url.Parse(navigate); err == nil {
			navigate = s.base.ResolveReference(u).String()
		}
	}

	b, err := json.Marshal(declarativeMessage{
		WebPush: declarativeWebPush,
		Notification: declarativeNotification{
			Title:    msg.Title,
			Body:     msg.Body,
			Navigate: navigate,
			Tag:      msg.Tag,
		},
	})
	if err != nil {
		panic(err)
	}
	return b
}
