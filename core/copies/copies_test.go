package copies

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/earthler/earthler/core/journal"
)

func TestKeyGoesToPaperAndBack(t *testing.T) {
	dir := t.TempDir()
	k, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadKey(dir)
	if err != nil || again != k {
		t.Fatal("the key changed between two loads")
	}
	text := k.Text()
	if len(text) != 52+12 {
		t.Fatalf("key text %q", text)
	}
	typed := " " + string(bytes.ToLower([]byte(text))) + "\n"
	parsed, err := ParseKey(typed)
	if err != nil || parsed != k {
		t.Fatalf("parsed %v, %v", parsed, err)
	}
	if _, err := ParseKey("abc"); err == nil {
		t.Fatal("a short text is not a key")
	}
}

func TestFilesSurviveTheTripAndRefuseChanges(t *testing.T) {
	blockSize = 1024
	defer func() { blockSize = 4 << 20 }()
	var key Key
	rand.Read(key[:])
	for _, size := range []int{0, 1, 1023, 1024, 1025, 3*1024 + 7} {
		content := make([]byte, size)
		rand.Read(content)
		var sealed bytes.Buffer
		if err := Encrypt(&sealed, "a", bytes.NewReader(content), key); err != nil {
			t.Fatal(err)
		}
		var back bytes.Buffer
		if err := Decrypt(&back, "a", bytes.NewReader(sealed.Bytes()), key); err != nil || !bytes.Equal(back.Bytes(), content) {
			t.Fatalf("size %d: %v", size, err)
		}
		raw := sealed.Bytes()
		changed := bytes.Clone(raw)
		changed[len(raw)-5] ^= 1
		bad := map[string][]byte{
			"a changed byte":     changed,
			"cut before the end": raw[:len(raw)-1],
		}
		if size >= blockSize {
			firstBlock := saltSize + blockSize + tagSize
			bad["cut at a block"] = raw[:firstBlock]
			bad["the first block repeated"] = append(append([]byte{}, raw[:firstBlock]...), raw[saltSize:]...)
		}
		for what, b := range bad {
			if err := Decrypt(&back, "a", bytes.NewReader(b), key); err == nil {
				t.Errorf("size %d, %s: accepted", size, what)
			}
		}
		var other Key
		rand.Read(other[:])
		if err := Decrypt(&back, "a", bytes.NewReader(raw), other); err == nil {
			t.Errorf("size %d: another key was accepted", size)
		}
		if err := Decrypt(&back, "b", bytes.NewReader(raw), key); err == nil {
			t.Errorf("size %d: another name was accepted", size)
		}
	}
}

// fill makes a data folder with a small journal and two secrets.
func fill(t *testing.T, dataDir string, entries int) {
	t.Helper()
	j, err := journal.Open(filepath.Join(dataDir, "journal"), journal.Options{SegmentSize: 300})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < entries; i++ {
		if _, err := j.Append([]byte(`{"n":` + string(rune('0'+i)) + `}`)); err != nil {
			t.Fatal(err)
		}
	}
	j.Close()
	os.MkdirAll(filepath.Join(dataDir, "secrets", "certificate"), 0o700)
	os.WriteFile(filepath.Join(dataDir, "secrets", "cookie.key"), []byte("cookie"), 0o600)
	os.WriteFile(filepath.Join(dataDir, "secrets", "certificate", "root.pem"), []byte("root"), 0o600)
}

func TestCopyKeepsUpAndRestoresWhole(t *testing.T) {
	root := t.TempDir()
	data, dest := filepath.Join(root, "data"), filepath.Join(root, "copy")
	fill(t, data, 5)
	key, err := LoadKey(filepath.Join(data, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	first, err := Copy(data, dest, key, now)
	if err != nil || first.Written != first.Files || first.Files < 4 {
		t.Fatalf("first copy: %+v, %v", first, err)
	}
	again, err := Copy(data, dest, key, now)
	if err != nil || again.Written != 0 || again.Files != first.Files {
		t.Fatalf("second copy: %+v, %v", again, err)
	}
	fill(t, data, 2) // one entry more in the last segment, and one in a new segment
	third, err := Copy(data, dest, key, now)
	if err != nil || third.Written != 2 || third.Files != first.Files+1 {
		t.Fatalf("third copy: %+v, %v", third, err)
	}
	files, _ := os.ReadDir(dest)
	if len(files) != third.Files+1 {
		t.Fatalf("the copy has %d files for %d sources", len(files), third.Files)
	}
	for _, f := range files {
		if raw, _ := os.ReadFile(filepath.Join(dest, f.Name())); bytes.Contains(raw, []byte(`"n":`)) || bytes.Contains(raw, []byte("cookie")) {
			t.Fatalf("%s is readable without the key", f.Name())
		}
	}
	if _, err := Check(dest, key); err != nil {
		t.Fatalf("check: %v", err)
	}

	restored := filepath.Join(root, "restored")
	rep, err := Restore(dest, restored, key)
	if err != nil || rep.Files != third.Files {
		t.Fatalf("restore: %+v, %v", rep, err)
	}
	paths, _ := sources(data)
	for _, p := range paths {
		a, _ := os.ReadFile(filepath.Join(data, filepath.FromSlash(p)))
		b, err := os.ReadFile(filepath.Join(restored, filepath.FromSlash(p)))
		if err != nil || !bytes.Equal(a, b) {
			t.Fatalf("%s came back different: %v", p, err)
		}
	}
	j, err := journal.Open(filepath.Join(restored, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
	if n, _ := j.Last(); n != 7 {
		t.Fatalf("the restored journal has %d entries", n)
	}
	if _, err := Restore(dest, restored, key); err == nil {
		t.Fatal("restoring over a folder with data should be refused")
	}
	var other Key
	rand.Read(other[:])
	if _, err := Check(dest, other); err == nil {
		t.Fatal("another key opened the copy")
	}

	// A file damaged in the copy is found by the check and stops a restore.
	for _, f := range files {
		if f.Name() != manifestName {
			path := filepath.Join(dest, f.Name())
			raw, _ := os.ReadFile(path)
			raw[len(raw)/2] ^= 1
			os.WriteFile(path, raw, 0o600)
			break
		}
	}
	if _, err := Check(dest, key); err == nil {
		t.Fatal("a damaged copy passed the check")
	}
	if _, err := Restore(dest, filepath.Join(root, "again"), key); err == nil {
		t.Fatal("a damaged copy was restored")
	}
}

// TestACopyNeverTouchesWhatIsNotACopy points the copy at folders that hold
// someone's files, the way a person does when they choose the root of a
// pen drive. A folder that holds other things and no copy is refused before
// anything is written. A folder that already has a copy keeps whatever else
// is in it, and only the copy's own old files are swept.
func TestACopyNeverTouchesWhatIsNotACopy(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	fill(t, data, 3)
	key, err := LoadKey(filepath.Join(data, "secrets"))
	if err != nil {
		t.Fatal(err)
	}

	drive := filepath.Join(root, "drive")
	os.MkdirAll(filepath.Join(drive, "Photos"), 0o700)
	photo := filepath.Join(drive, "family-photo.jpg")
	os.WriteFile(photo, []byte("precious"), 0o600)
	before, _ := os.ReadDir(drive)
	if _, err := Copy(data, drive, key, time.Now()); err == nil {
		t.Fatal("a folder that holds other things was accepted for a first copy")
	}
	if after, _ := os.ReadDir(drive); len(after) != len(before) {
		t.Fatalf("a refused copy left %d things in the folder, there were %d", len(after), len(before))
	}
	if got, err := os.ReadFile(photo); err != nil || string(got) != "precious" {
		t.Fatalf("the photo: %q, %v", got, err)
	}

	// The leftovers of a first copy that was cut short are ours, not foreign.
	dest := filepath.Join(root, "copy")
	os.MkdirAll(dest, 0o700)
	os.WriteFile(filepath.Join(dest, "0123456789abcdef0123456789abcdef"), []byte("half"), 0o600)
	os.WriteFile(filepath.Join(dest, "manifest.tmp"), []byte("half"), 0o600)
	if _, err := Copy(data, dest, key, time.Now()); err != nil {
		t.Fatalf("a first copy after one that was cut short: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "0123456789abcdef0123456789abcdef")); err == nil {
		t.Fatal("the leftover of the cut copy was not swept")
	}

	// A copy that exists tolerates the things the system puts in a folder.
	finder := filepath.Join(dest, ".DS_Store")
	os.WriteFile(finder, []byte("finder"), 0o600)
	fill(t, data, 2)
	if _, err := Copy(data, dest, key, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(finder); err != nil || string(got) != "finder" {
		t.Fatalf("a file that is not the copy's was touched: %q, %v", got, err)
	}
	if _, err := Check(dest, key); err != nil {
		t.Fatal(err)
	}
}
