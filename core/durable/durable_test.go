package durable

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileReplacesWholeAndLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("two, longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "two, longer" {
		t.Fatalf("read %q, %v", got, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", info.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("%d files in the folder, want only the one that was written", len(entries))
	}
}

func TestWriteFileFailsCleanlyWhenTheFolderIsMissing(t *testing.T) {
	if err := WriteFile(filepath.Join(t.TempDir(), "missing", "secret"), []byte("x"), 0o600); err == nil {
		t.Fatal("writing into a folder that does not exist worked")
	}
}
