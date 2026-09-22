package sandcat

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// The generated .devcontainer tree is copied wholesale from the embedded
// files, so an embed pattern that misses a file would surface as a silently
// incomplete sandbox rather than a build error. Compare the embedded tree
// against the on-disk template directory file-for-file, so a template added
// upstream is covered the moment it lands rather than when someone remembers
// to extend a list.
func TestTemplatesMatchDisk(t *testing.T) {
	onDisk := map[string][]byte{}
	err := filepath.WalkDir("cli/templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("cli/templates", p)
		onDisk[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Skipf("cli/templates not on disk (post-cutover?): %v", err)
	}
	if len(onDisk) == 0 {
		t.Fatal("no template files found on disk")
	}

	embedded := map[string]bool{}
	err = fs.WalkDir(Templates, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		embedded[p] = true
		want, ok := onDisk[p]
		if !ok {
			t.Errorf("%s is embedded but not on disk", p)
			return nil
		}
		got, err := fs.ReadFile(Templates, p)
		if err != nil {
			return err
		}
		if string(got) != string(want) {
			t.Errorf("%s: embedded bytes differ from disk", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for p := range onDisk {
		if !embedded[p] {
			t.Errorf("%s is on disk but not embedded", p)
		}
	}
}

// fs.Sub re-roots the tree; callers must not have to spell cli/templates.
func TestTemplatesAreReRooted(t *testing.T) {
	if _, err := fs.Stat(Templates, "cli/templates"); err == nil {
		t.Error("Templates still exposes the cli/templates prefix")
	}
	if _, err := fs.Stat(Templates, "devcontainer/compose-all.yml"); err != nil {
		t.Errorf("expected devcontainer/compose-all.yml at the root: %v", err)
	}
}
