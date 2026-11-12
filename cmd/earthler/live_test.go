package main

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// listen opens the live connection of the conversation, as the page does.
func (v *visitor) listen(s *site, after string) (*bufio.Scanner, func()) {
	v.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", s.srv.URL+"/chat/live?after="+after, nil)
	res, err := v.client.Do(req)
	if err != nil {
		v.t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		v.t.Fatalf("live: status %d, type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	return bufio.NewScanner(res.Body), func() { cancel(); res.Body.Close() }
}

// block reads one event from the stream, skipping the reconnection hint
// and the heartbeats.
func block(t *testing.T, sc *bufio.Scanner) string {
	t.Helper()
	var lines []string
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			text := strings.Join(lines, "\n")
			lines = nil
			if strings.HasPrefix(text, "retry:") || strings.HasPrefix(text, ":") {
				continue
			}
			return text
		}
		lines = append(lines, line)
	}
	t.Fatalf("the live connection ended: %v", sc.Err())
	return ""
}

func (v *visitor) remove(s *site, number string) (int, string) {
	return v.post(s, "/chat/delete", s.srv.URL, url.Values{"number": {number}})
}

// TestMessagesArriveLiveAndCanBeDeleted is the proof of live updates and
// deletion: a page that is open sees a message the moment it is written,
// sees it go when its author deletes it, and the text is gone from the
// screens, from the search and, after the night's work, from the journal.
func TestMessagesArriveLiveAndCanBeDeleted(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)
	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")

	stream, stop := bia.listen(s, "0")
	defer stop()
	ana.say(s, garden)
	got := block(t, stream)
	wantIn(t, got, "event: posted", "the live event")
	wantIn(t, got, "id: 5", "the entry number of the message")
	wantIn(t, got, garden, "the text in the live event")
	wantIn(t, got, `data-account="3"`, "the author in the live event")

	// Only the author may delete, and the deletion reaches the open page.
	if status, _ := bia.remove(s, "5"); status != http.StatusForbidden {
		t.Fatalf("deleting someone else's message: status %d", status)
	}
	if status, _ := ana.remove(s, "70"); status != http.StatusNotFound {
		t.Fatalf("deleting a message that does not exist: status %d", status)
	}
	_, body := ana.remove(s, "5")
	wantOut(t, body, garden, "the channel after deleting")
	wantIn(t, body, "Mensagem apagada.", "the channel after deleting")
	if got := block(t, stream); got != "id: 6\nevent: deleted\ndata: 5" {
		t.Fatalf("the deletion event was %q", got)
	}
	_, body = bia.get(s, "/search?q=agua")
	wantOut(t, body, garden, "the search after deleting")
	stop()

	// A page that comes back late catches up: the deleted message, and a
	// deletion of a message it already had.
	ana.say(s, "Alguém viu a enxada?")
	late, stopLate := bia.listen(s, "0")
	got = block(t, late)
	wantIn(t, got, "event: posted", "catching up")
	wantIn(t, got, "Mensagem apagada.", "catching up on a deleted message")
	wantOut(t, got, garden, "catching up on a deleted message")
	wantIn(t, block(t, late), "enxada", "catching up on a new message")
	stopLate()
	known, stopKnown := bia.listen(s, "5")
	if got := block(t, known); got != "id: 6\nevent: deleted\ndata: 5" {
		t.Fatalf("a page that had the message catches up with %q", got)
	}
	stopKnown()

	// The journal still holds the text until the erasure runs. After it, the
	// text is gone from the files, the chain is whole, and a rebuilt index
	// still says a message was deleted.
	segments, _ := filepath.Glob(filepath.Join(dir, "journal", "*.journal"))
	raw, _ := os.ReadFile(segments[0])
	wantIn(t, string(raw), "horta", "the journal before the erasure")
	n, err := s.app.erase(context.Background(), time.Now())
	if err != nil || n != 1 {
		t.Fatalf("erase: %d entries, %v", n, err)
	}
	raw, _ = os.ReadFile(segments[0])
	wantOut(t, string(raw), "horta", "the journal after the erasure")
	if err := s.app.journal.Verify(); err != nil {
		t.Fatal(err)
	}
	if n, err := s.app.erase(context.Background(), time.Now()); err != nil || n != 0 {
		t.Fatalf("a second erasure: %d entries, %v", n, err)
	}
	s.stop()
	if err := os.RemoveAll(filepath.Join(dir, "indexes")); err != nil {
		t.Fatal(err)
	}
	s = startSite(t, dir)
	_, body = bia.get(s, "/chat")
	wantIn(t, body, "Mensagem apagada.", "the channel after rebuilding the indexes")
	wantIn(t, body, "enxada", "the channel after rebuilding the indexes")
	wantOut(t, body, garden, "the channel after rebuilding the indexes")
}
