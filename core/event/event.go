// Package event defines what goes inside a journal entry: who did what, and
// when. The journal itself only sees one JSON object per entry.
package event

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Version is the version of the event layout written by this code.
const Version = 1

// Event is the content of one journal entry.
type Event struct {
	// V is the version of this layout.
	V int `json:"v"`

	// Type says what happened, as a noun and a verb in the past, such as
	// "message.created". Types are written in the journal forever, so a type
	// that was once used is never given another meaning.
	Type string `json:"type"`

	// Time is the server clock when the event was written, in milliseconds
	// since 1970. It is for showing to people. The order of events is the
	// order of the entries, never this field.
	Time int64 `json:"time"`

	// Account and Device are the entry numbers of the account and of the
	// device that acted. Zero means the system itself.
	Account uint64 `json:"account,omitempty"`
	Device  uint64 `json:"device,omitempty"`

	// Salt is random. It goes away together with the content when an entry
	// is erased, so the digest that stays cannot be used to confirm a guess
	// of what the erased text was.
	Salt string `json:"salt"`

	// Data depends on the type.
	Data json.RawMessage `json:"data"`
}

var validType = regexp.MustCompile(`^[a-z]+(\.[a-z]+)+$`)

// New builds the content of an entry.
func New(typ string, account, device uint64, now time.Time, data any) ([]byte, error) {
	if !validType.MatchString(typ) {
		return nil, fmt.Errorf("event: %q is not a valid type", typ)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("event %s: %w", typ, err)
	}
	if len(raw) == 0 || raw[0] != '{' {
		return nil, fmt.Errorf("event %s: data must be a JSON object", typ)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return json.Marshal(Event{
		V:       Version,
		Type:    typ,
		Time:    now.UnixMilli(),
		Account: account,
		Device:  device,
		Salt:    base64.RawURLEncoding.EncodeToString(salt),
		Data:    raw,
	})
}

// Decode reads the content of an entry.
func Decode(content []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(content, &e); err != nil {
		return e, fmt.Errorf("event: %w", err)
	}
	if e.Type == "" {
		return e, errors.New("event: no type")
	}
	return e, nil
}

// When returns Time as a time value.
func (e Event) When() time.Time { return time.UnixMilli(e.Time) }
