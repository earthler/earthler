package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
)

// Each test here is one defect that an independent review of the code
// found, written first as the scenario that shows it.

// TestAMessageCannotWriteToTheLiveStream posts a text built to look like
// fields of the stream. Every open page must receive a normal new message
// with the number of the real entry, not a reload and not a forged number.
func TestAMessageCannotWriteToTheLiveStream(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")
	stream, stop := bia.listen(s, "0")
	defer stop()

	ana.say(s, "oi\rid: 99999999\revent: reload\rretry: 99999999\rdata: x")
	got := block(t, stream)
	asBrowser := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(got)
	var ids, events []string
	for _, line := range strings.Split(asBrowser, "\n") {
		name, value, _ := strings.Cut(line, ": ")
		switch name {
		case "id":
			ids = append(ids, value)
		case "event":
			events = append(events, value)
		case "data":
		default:
			t.Fatalf("an unknown field %q reached the page:\n%q", name, got)
		}
	}
	if len(ids) != 1 || ids[0] != "5" || len(events) != 1 || events[0] != "posted" {
		t.Fatalf("the page read the ids %v and the events %v, want the entry 5 and one new message:\n%q", ids, events, got)
	}
}

// TestAFileSentToDeleteIsNeverRead sends a large multipart body to a route
// that only wants a number. The server must not read it, because a file
// part is kept in the temporary folder and could fill the disk of the
// journal.
func TestAFileSentToDeleteIsNeverRead(t *testing.T) {
	s := startSite(t, t.TempDir())
	v := newVisitor(t)
	v.signUp(s, "Ana")

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("junk", "junk.bin")
	fw.Write(make([]byte, 20<<20))
	mw.WriteField("number", "1")
	mw.Close()
	size := int64(body.Len())
	var read atomic.Int64
	req, _ := http.NewRequest("POST", s.srv.URL+"/chat/delete", countingReader{&body, &read})
	req.ContentLength = size
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", s.srv.URL)
	res, err := v.client.Do(req)
	if err != nil {
		// The server may close the connection once it has answered.
		t.Logf("the server answered by closing the connection: %v", err)
	} else {
		res.Body.Close()
		if res.StatusCode < 400 {
			t.Fatalf("status %d for a body with a file", res.StatusCode)
		}
	}
	time.Sleep(200 * time.Millisecond) // the client may still be sending when the server answered
	if got := read.Load(); got > 1<<20 {
		t.Fatalf("the server read %d MB of a body it was never going to use", got>>20)
	}
}

type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// TestSigningUpIsAllOrNothing signs residents up with a connection that is
// cut at a random moment. Whatever happens, no account may exist without a
// device, and a name whose sign up failed must be free to try again.
func TestSigningUpIsAllOrNothing(t *testing.T) {
	s := startSite(t, t.TempDir())
	svc := accounts.New(s.app.byName["accounts"], s.app.scribe, make([]byte, 32))
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 60; i++ {
		name := fmt.Sprintf("Pessoa %d", i)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rng.Intn(2000))*time.Microsecond)
		_, err := svc.SignUp(ctx, name, false)
		cancel()
		if err != nil {
			if _, err := svc.SignUp(context.Background(), name, false); err != nil {
				t.Fatalf("%q failed once and then failed again with %v: the name was left taken", name, err)
			}
		}
	}
	var orphans int
	err := s.app.byName["accounts"].DB().QueryRow(`SELECT count(*) FROM accounts a WHERE NOT EXISTS (SELECT 1 FROM devices d WHERE d.account = a.number)`).Scan(&orphans)
	if err != nil || orphans != 0 {
		t.Fatalf("%d accounts have no device, %v", orphans, err)
	}
}

// TestAccountsWrittenInTwoEntriesStillWork keeps faith with the journals
// written before an account and its first device became one entry.
func TestAccountsWrittenInTwoEntriesStillWork(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)
	now := time.Now()
	content, _ := event.New(event.AccountCreated, 0, 0, now, event.AccountCreatedData{Name: "Antiga"})
	entries, err := s.app.scribe.Write(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	account := entries[0].Number
	content, _ = event.New(event.DeviceCreated, account, 0, now, event.DeviceCreatedData{Account: account})
	entries, err = s.app.scribe.Write(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	device := entries[0].Number

	key, err := accounts.LoadKey(filepath.Join(dir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	value := fmt.Sprintf("%d.0", device)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	u, _ := url.Parse(s.srv.URL)
	old := newVisitor(t)
	old.client.Jar.SetCookies(u, []*http.Cookie{{Name: "earthler", Value: value + "." + hex.EncodeToString(mac.Sum(nil))}})
	_, body := old.get(s, "/chat")
	wantIn(t, body, "Antiga", "the page of an account from the two entry format")
}

// TestDeletedMessagesAreTheSameAfterARebuild deletes a message, lets the
// night's erasure take its text out of the journal and then rebuilds the
// index. The rebuilt row must be the one that was kept up to date, time
// included.
func TestDeletedMessagesAreTheSameAfterARebuild(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	ana.say(s, "vai embora")
	time.Sleep(1100 * time.Millisecond) // the clock of an entry ticks in seconds
	ana.say(s, "fica")
	if status, _ := ana.remove(s, "4"); status >= 400 {
		t.Fatalf("deleting: status %d", status)
	}
	if n, err := s.app.erase(context.Background(), time.Now()); err != nil || n != 1 {
		t.Fatalf("erase: %d, %v", n, err)
	}
	kept := chatDump(t, s)

	s.stop()
	os.RemoveAll(filepath.Join(dir, "indexes"))
	s = startSite(t, dir)
	if got := chatDump(t, s); got != kept {
		t.Fatalf("the rebuilt chat differs from the one kept up to date:\n%s", firstDifference(kept, got))
	}
}

// TestAFailingDatabaseIsNotASignedOutVisitor closes the accounts index, as a
// disk error would, and asks for a page. A resident who gets a failure must
// be told the system is failing, not sent to the sign up screen as if they
// had been signed out.
func TestAFailingDatabaseIsNotASignedOutVisitor(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := quiet(t)
	ana.signUp(s, "Ana")
	s.app.byName["accounts"].Close()

	req, _ := http.NewRequest("GET", s.srv.URL+"/chat", nil)
	res, err := ana.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d with Location %q, want 503", res.StatusCode, res.Header.Get("Location"))
	}
}
