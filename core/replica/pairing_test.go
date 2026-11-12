package replica

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)

func registry(t *testing.T) (*Registry, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "replication")
	r, err := OpenRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, dir
}

func TestACodePairsOneMachineOnce(t *testing.T) {
	r, dir := registry(t)
	code, err := NewCode(dir, t0)
	if err != nil || len(code) != 14 || strings.Count(code, "-") != 2 {
		t.Fatalf("code %q, %v", code, err)
	}

	// Typed the way a person types it: lower case, with spaces.
	typed := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	secret, err := r.Redeem(typed, "the machine at Ana's", "10.0.0.7", t0.Add(time.Minute))
	if err != nil || len(secret) < 40 {
		t.Fatalf("redeem: %q, %v", secret, err)
	}
	if _, err := r.Redeem(code, "again", "10.0.0.7", t0.Add(2*time.Minute)); err != ErrBadCode {
		t.Fatalf("a code used twice: %v", err)
	}
	if id, ok := r.Authorize(secret, "10.0.0.7", t0.Add(3*time.Minute)); !ok || id == "" {
		t.Fatal("the token the code gave was refused")
	}
	if _, ok := r.Authorize(secret+"x", "10.0.0.7", t0.Add(3*time.Minute)); ok {
		t.Fatal("a token that is not one was accepted")
	}
	if _, ok := r.Authorize("", "10.0.0.7", t0.Add(3*time.Minute)); ok {
		t.Fatal("an empty token was accepted")
	}
	machines, err := r.Machines()
	if err != nil || len(machines) != 1 || machines[0].Label != "the machine at Ana's" {
		t.Fatalf("machines: %+v, %v", machines, err)
	}
}

func TestOnlyHashesAreWritten(t *testing.T) {
	r, dir := registry(t)
	code, _ := NewCode(dir, t0)
	secret, err := r.Redeem(code, "x", "10.0.0.7", t0)
	if err != nil {
		t.Fatal(err)
	}
	// The pairing file is gone, and the secret is nowhere on the disk.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(raw), secret) || strings.Contains(string(raw), strings.ReplaceAll(code, "-", "")) {
			t.Fatalf("%s holds a secret in the clear", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, pairingFile)); err == nil {
		t.Fatal("the pairing file is still there after it was used")
	}
}

func TestACodeExpires(t *testing.T) {
	r, dir := registry(t)
	code, _ := NewCode(dir, t0)
	if _, err := r.Redeem(code, "late", "10.0.0.7", t0.Add(CodeLife+time.Second)); err != ErrBadCode {
		t.Fatalf("an expired code: %v", err)
	}
	// A new code replaces the old one, which can no longer be used.
	first, _ := NewCode(dir, t0)
	second, _ := NewCode(dir, t0)
	if _, err := r.Redeem(first, "x", "10.0.0.7", t0); err != ErrBadCode {
		t.Fatalf("the replaced code: %v", err)
	}
	if _, err := r.Redeem(second, "x", "10.0.0.7", t0); err != nil {
		t.Fatalf("the new code: %v", err)
	}
}

func TestGuessingIsStopped(t *testing.T) {
	r, dir := registry(t)
	code, _ := NewCode(dir, t0)
	for i := 0; i < maxFailures; i++ {
		if _, err := r.Redeem("AAAA-AAAA-AAAA", "x", "10.0.0.9", t0.Add(time.Duration(i)*time.Second)); err != ErrBadCode {
			t.Fatalf("guess %d: %v", i, err)
		}
	}
	// Even the right code is refused from the one who guessed, for a minute.
	if _, err := r.Redeem(code, "x", "10.0.0.9", t0.Add(10*time.Second)); err != ErrTooMany {
		t.Fatalf("after the guesses: %v", err)
	}
	// Someone else is not locked out by it.
	if _, err := r.Redeem(code, "x", "10.0.0.8", t0.Add(10*time.Second)); err != nil {
		t.Fatalf("another address: %v", err)
	}
}

func TestTokensCanBeRevoked(t *testing.T) {
	r, dir := registry(t)
	code, _ := NewCode(dir, t0)
	secret, _ := r.Redeem(code, "x", "10.0.0.7", t0)
	id, ok := r.Authorize(secret, "10.0.0.7", t0)
	if !ok {
		t.Fatal("not authorized")
	}
	if err := r.Revoke(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Authorize(secret, "10.0.0.7", t0); ok {
		t.Fatal("a revoked token still works")
	}
}

func TestWrongTokensAreCountedToo(t *testing.T) {
	r, _ := registry(t)
	for i := 0; i < maxFailures; i++ {
		r.Authorize("not a token", "10.0.0.9", t0)
	}
	if r.tooMany("10.0.0.9", t0, false) != true {
		t.Fatal("repeated wrong tokens are not limited")
	}
	if r.tooMany("10.0.0.9", t0.Add(failureSpan+time.Second), false) {
		t.Fatal("the limit never ends")
	}
}
