package accounts

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheSessionKeyIsMadeOnceAndNeverReplacedByAccident(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets")
	first, err := LoadKey(dir)
	if err != nil || len(first) != 32 {
		t.Fatalf("first load: %d bytes, %v", len(first), err)
	}
	again, err := LoadKey(dir)
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("second load: %v, same key: %v", err, bytes.Equal(first, again))
	}

	// A file that is not a key, such as one a power cut left empty, stops
	// the server instead of silently signing everyone out.
	path := filepath.Join(dir, "session.key")
	for _, damaged := range [][]byte{{}, []byte("short"), bytes.Repeat([]byte{1}, 33)} {
		if err := os.WriteFile(path, damaged, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadKey(dir); err == nil || !strings.Contains(err.Error(), "not a session key") {
			t.Fatalf("a key file of %d bytes: %v", len(damaged), err)
		}
		if got, _ := os.ReadFile(path); len(got) != len(damaged) {
			t.Fatalf("the damaged file was replaced by one of %d bytes", len(got))
		}
	}
}

func TestOnlyVisibleCharactersMakeAName(t *testing.T) {
	for _, ok := range []string{"Ana", "José da Silva", "Zoë", "Müller", "李雷", "O'Brien", "Ana-Lúcia", "Ana 2", "🌱", "Ana (horta)"} {
		if err := checkName(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"Ana\u200b", "Ana\u200d", "Ana\u202e", "Ana\u2066", "A\u00adna", "An\u2060a", "\ufeffAna", "Ana\ue000",
		"Ana\xff", "Ana\xed\xa0\x80", "Ana\ufffd", "Jose\u0301", "Ana\u0000", "Ana\u0007", "Ana\u2028", "Ana\u00a0Lima",
	} {
		if err := checkName(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
