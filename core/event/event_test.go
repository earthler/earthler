package event

import (
	"testing"
	"time"

	"github.com/earthler/earthler/core/journal"
)

func TestNewAndDecode(t *testing.T) {
	now := time.UnixMilli(1762640000123)
	content, err := New("message.created", 104, 221, now, map[string]any{"channel": 37, "text": "Bom dia"})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.CheckContent(content); err != nil {
		t.Fatalf("the journal would refuse this event: %v", err)
	}
	e, err := Decode(content)
	if err != nil {
		t.Fatal(err)
	}
	if e.V != Version || e.Type != "message.created" || e.Account != 104 || e.Device != 221 || !e.When().Equal(now) {
		t.Fatalf("decoded %+v", e)
	}
	if string(e.Data) != `{"channel":37,"text":"Bom dia"}` {
		t.Fatalf("data = %s", e.Data)
	}
}

func TestSaltIsDifferentEveryTime(t *testing.T) {
	now := time.Now()
	a, _ := New("message.created", 1, 1, now, map[string]string{"text": "same"})
	b, _ := New("message.created", 1, 1, now, map[string]string{"text": "same"})
	if string(a) == string(b) {
		t.Fatal("two events with the same text have the same bytes, so a digest would confirm a guess")
	}
	ea, _ := Decode(a)
	if len(ea.Salt) < 20 {
		t.Fatalf("salt is too short: %q", ea.Salt)
	}
}

func TestBadEventsAreRefused(t *testing.T) {
	now := time.Now()
	for _, typ := range []string{"", "message", "Message.Created", "message.", "message created", "msg.nova!"} {
		if _, err := New(typ, 0, 0, now, map[string]string{}); err == nil {
			t.Errorf("type %q was accepted", typ)
		}
	}
	if _, err := New("message.created", 0, 0, now, "just text"); err == nil {
		t.Error("data that is not an object was accepted")
	}
	if _, err := Decode([]byte(`{"v":1}`)); err == nil {
		t.Error("an event with no type was decoded")
	}
}
