package gitignore

import (
	"github.com/jehoctor/snadcat/internal/testutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitProject makes a temp dir that looks like a git working tree, optionally
// seeded with .gitignore content.
func gitProject(t *testing.T, existing string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func read(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpdateCreatesGitignore(t *testing.T) {
	dir := gitProject(t, "")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	got := read(t, dir)
	if !strings.HasPrefix(got, StartMarker+"\n") {
		t.Errorf("block should start the file, got %q", got)
	}
	for _, want := range []string{".devcontainer/*", "!.devcontainer/devbox.tools.json", ".snadcat/settings.local.json", EndMarker} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

// A missing trailing newline on the user's last rule must not glue our marker
// onto it — that would silently corrupt their final pattern.
func TestUpdateSeparatesFromUnterminatedLastLine(t *testing.T) {
	dir := gitProject(t, "*.log")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	got := read(t, dir)
	if !strings.HasPrefix(got, "*.log\n\n"+StartMarker) {
		t.Errorf("got %q", got)
	}
}

func TestUpdateAppendsAfterExistingRules(t *testing.T) {
	dir := gitProject(t, "node_modules/\n")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	got := read(t, dir)
	if !strings.HasPrefix(got, "node_modules/\n\n"+StartMarker) {
		t.Errorf("got %q", got)
	}
}

func TestUpdateIsIdempotent(t *testing.T) {
	dir := gitProject(t, "node_modules/\n")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	first := read(t, dir)
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	if second := read(t, dir); second != first {
		t.Errorf("second Update changed the file:\n%q\nvs\n%q", second, first)
	}
}

// Not a git working tree — there is nothing to gitignore into.
func TestUpdateSkipsNonGitProject(t *testing.T) {
	dir := t.TempDir()
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Error(".gitignore should not have been created")
	}
}

func TestRemoveBlockRestoresUserRules(t *testing.T) {
	dir := gitProject(t, "node_modules/\n")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveBlock(dir); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir); got != "node_modules/\n" {
		t.Errorf("got %q, want %q", got, "node_modules/\n")
	}
}

// When the block was all there was, the file goes too — mirroring the
// pre-init state rather than leaving an empty .gitignore behind.
func TestRemoveBlockDeletesFileWhenItWasTheOnlyContent(t *testing.T) {
	dir := gitProject(t, "")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveBlock(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Error(".gitignore should have been removed")
	}
}

func TestRemoveBlockPreservesRulesAfterTheBlock(t *testing.T) {
	dir := gitProject(t, "")
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".gitignore")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, []byte("after.txt\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveBlock(dir); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir); got != "after.txt\n" {
		t.Errorf("got %q, want %q", got, "after.txt\n")
	}
}

func TestRemoveBlockIsNoopWithoutMarker(t *testing.T) {
	dir := gitProject(t, "node_modules/\n")
	if err := RemoveBlock(dir); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir); got != "node_modules/\n" {
		t.Errorf("got %q", got)
	}
}

// Repeated enable/disable cycles must converge rather than accumulate blank
// lines where the block used to be.
func TestUpdateRemoveCyclesConverge(t *testing.T) {
	dir := gitProject(t, "node_modules/\n")
	for i := 0; i < 3; i++ {
		if err := Update(dir); err != nil {
			t.Fatal(err)
		}
		if err := RemoveBlock(dir); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, dir); got != "node_modules/\n" {
		t.Errorf("got %q, want %q", got, "node_modules/\n")
	}
}

func TestHasBlock(t *testing.T) {
	dir := gitProject(t, "node_modules/\n")
	if HasBlock(dir) {
		t.Error("no block yet")
	}
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	if !HasBlock(dir) {
		t.Error("block should be detected")
	}
}

// The block must be byte-identical to what the bash implementation writes;
// it ends up committed in users' repos.
func TestBlockMatchesBashOutput(t *testing.T) {
	libdir, err := filepath.Abs("../../cli/lib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(libdir, "gitignore.bash")); err != nil {
		t.Skip("bash originals unavailable")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c",
		`set -euo pipefail; source "$SCT_LIBDIR/gitignore.bash"; update_gitignore "$1"; cat "$1/.gitignore"`,
		"bash", dir)
	cmd.Env = append(os.Environ(), "SCT_LIBDIR="+libdir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}

	goDir := gitProject(t, "")
	if err := Update(goDir); err != nil {
		t.Fatal(err)
	}
	if got := testutil.Normalize(read(t, goDir)); got != string(out) {
		t.Errorf("block differs from bash:\n got: %q\nwant: %q", got, out)
	}
}
