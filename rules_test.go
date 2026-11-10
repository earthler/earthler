// Package rules holds no code. Its tests enforce the rules that keep this
// repository readable by someone, or something, with a small memory and no
// internet: a successor that will maintain a community's system alone.
package rules

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	module   = "github.com/earthler/earthler"
	maxLines = 300
)

// goFiles lists every Go file of the repository.
func goFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") && path != "." || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestFilesAreShort keeps every file small enough to be read whole.
func TestFilesAreShort(t *testing.T) {
	for _, path := range goFiles(t) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		lines := 0
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			lines++
		}
		f.Close()
		if lines > maxLines {
			t.Errorf("%s has %d lines, the limit is %d: split it", path, lines, maxLines)
		}
	}
}

// TestLayersPointOneWay keeps the shape of the code: a screen talks to the
// core and never to another screen, the core knows no screen, and the
// experiments stay apart from the system.
func TestLayersPointOneWay(t *testing.T) {
	for _, path := range goFiles(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if !strings.HasPrefix(imported, module+"/") {
				continue
			}
			target := strings.TrimPrefix(imported, module+"/")
			switch {
			case strings.HasPrefix(target, "experiments/") || strings.HasPrefix(target, "cmd/"):
				t.Errorf("%s imports %s: nothing may import an experiment or a command", path, target)
			case strings.HasPrefix(dir, "core/") && strings.HasPrefix(target, "screens/"):
				t.Errorf("%s imports %s: the core must not know the screens", path, target)
			case strings.HasPrefix(dir, "screens/") && strings.HasPrefix(target, "screens/") && target != dir:
				t.Errorf("%s imports %s: a screen must not call another screen", path, target)
			case strings.HasPrefix(dir, "experiments/"):
				t.Errorf("%s imports %s: an experiment must not depend on the system", path, target)
			}
		}
	}
}
