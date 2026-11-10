package live

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// listen opens a connection and returns a reader of its lines.
func listen(t *testing.T, url string) (*bufio.Scanner, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", res.Header.Get("Content-Type"))
	}
	return bufio.NewScanner(res.Body), func() { cancel(); res.Body.Close() }
}

// next reads lines until a blank line and returns them joined.
func next(t *testing.T, sc *bufio.Scanner) string {
	t.Helper()
	var lines []string
	for sc.Scan() {
		if sc.Text() == "" {
			return strings.Join(lines, "\n")
		}
		lines = append(lines, sc.Text())
	}
	t.Fatalf("the stream ended: %v", sc.Err())
	return ""
}

func TestMessagesReachEveryPage(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := After(r)
		hub.Serve(w, r, func(send func(Message)) error {
			send(Message{ID: after + 1, Event: "posted", Data: "catch up\nsecond line"})
			return nil
		})
	}))
	defer srv.Close()

	a, stopA := listen(t, srv.URL+"/?after=10")
	defer stopA()
	b, stopB := listen(t, srv.URL)
	defer stopB()
	if got := next(t, a); got != "retry: 2000" {
		t.Fatalf("first block %q", got)
	}
	if got := next(t, a); got != "id: 11\nevent: posted\ndata: catch up\ndata: second line" {
		t.Fatalf("catch up %q", got)
	}
	next(t, b)
	if got := next(t, b); !strings.HasPrefix(got, "id: 1\n") {
		t.Fatalf("catch up without after %q", got)
	}
	for hub.Connections() < 2 {
		time.Sleep(time.Millisecond)
	}
	hub.Send(Message{ID: 12, Event: "deleted", Data: "7"})
	for _, sc := range []*bufio.Scanner{a, b} {
		if got := next(t, sc); got != "id: 12\nevent: deleted\ndata: 7" {
			t.Fatalf("sent %q", got)
		}
	}
	stopA()
	for hub.Connections() != 1 {
		time.Sleep(time.Millisecond)
	}
}

func TestSlowPageIsDroppedAndHeartbeatKeepsTheOthers(t *testing.T) {
	heartbeat = 20 * time.Millisecond
	defer func() { heartbeat = 25 * time.Second }()
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.Serve(w, r, func(func(Message)) error { return nil })
	}))
	defer srv.Close()
	sc, stop := listen(t, srv.URL)
	defer stop()
	next(t, sc)
	if got := next(t, sc); got != ": alive" {
		t.Fatalf("heartbeat %q", got)
	}
	for hub.Connections() < 1 {
		time.Sleep(time.Millisecond)
	}
	// Nobody reads, so the queue fills and the connection is dropped.
	for i := 0; i < queue*4; i++ {
		hub.Send(Message{ID: uint64(i + 1), Event: "posted", Data: strings.Repeat("x", 1000)})
	}
	if hub.Connections() != 0 {
		t.Fatalf("%d connections kept after the queue overflowed", hub.Connections())
	}
	for sc.Scan() {
	}
	if sc.Err() != nil && !strings.Contains(sc.Err().Error(), "EOF") {
		t.Logf("the stream ended with: %v", sc.Err())
	}
}

func TestEncode(t *testing.T) {
	if got := string(encode(Message{Event: "reload", Data: ""})); got != "event: reload\ndata: \n\n" {
		t.Fatalf("%q", got)
	}
}
