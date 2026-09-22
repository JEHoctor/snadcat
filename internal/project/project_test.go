package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/tmp/demo", "demo-sandbox"},
		{"/tmp/demo/", "demo-sandbox"},
		{"/a/b/my-project", "my-project-sandbox"},
	}
	for _, tc := range tests {
		if got := DeriveName(tc.in); got != tc.want {
			t.Errorf("DeriveName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Any one of the three markers is enough; a project that hasn't been init'd
// still resolves to its git root, which is where init should write.
func TestFindRootDetectsEachMarker(t *testing.T) {
	for _, marker := range []string{Dir, ".git", ".devcontainer"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, marker), 0o755); err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(root, "a", "b")
			if err := os.MkdirAll(nested, 0o755); err != nil {
				t.Fatal(err)
			}

			got, err := FindRoot(nested)
			if err != nil {
				t.Fatal(err)
			}
			// t.TempDir can sit under a symlinked path (/tmp -> /private/tmp
			// on macOS), so compare resolved paths.
			wantResolved, _ := filepath.EvalSymlinks(root)
			gotResolved, _ := filepath.EvalSymlinks(got)
			if gotResolved != wantResolved {
				t.Errorf("got %q, want %q", got, root)
			}
		})
	}
}

// A .git *file* is what git writes in worktrees and submodules; presence is
// what matters, not whether it is a directory.
func TestFindRootAcceptsGitFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FindRoot(root); err != nil {
		t.Errorf("a .git file should count as a root marker: %v", err)
	}
}

// The walk has to terminate at the filesystem root rather than spin.
func TestFindRootErrorsWhenNoMarkerExists(t *testing.T) {
	// A temp dir with no markers; parents up to / have none either, unless
	// the test tree itself is inside a repo — so use a deliberately isolated
	// subtree under the temp dir.
	dir := filepath.Join(t.TempDir(), "x", "y")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := FindRoot(dir); err == nil {
		t.Skip("temp dir is itself inside a marked tree")
	} else if !strings.Contains(err.Error(), "repository root not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindComposeFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Marker present but no compose file yet — the error should point at
	// init rather than at a missing directory.
	_, err := FindComposeFile(root)
	if err == nil {
		t.Fatal("expected an error before init has run")
	}
	if !strings.Contains(err.Error(), "sandcat init") {
		t.Errorf("error should point at init, got %v", err)
	}

	path := filepath.Join(root, ComposeFile)
	if err := os.WriteFile(path, []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindComposeFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "compose-all.yml" {
		t.Errorf("got %q", got)
	}
}

func TestVerifyRelativePath(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyRelativePath(base, "settings.json"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := VerifyRelativePath(base, "/etc/passwd"); err == nil {
		t.Error("absolute paths must be rejected")
	}
	if err := VerifyRelativePath(base, "missing.json"); err == nil {
		t.Error("missing files must be rejected")
	}
	if err := VerifyRelativePath(filepath.Join(base, "nope"), "settings.json"); err == nil {
		t.Error("a non-directory base must be rejected")
	}
}
