package initialize

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jehoctor/snadcat/internal/gitignore"
	"github.com/jehoctor/snadcat/internal/log"
)

func quiet(t *testing.T) {
	t.Helper()
	old := log.Out
	log.Out = &bytes.Buffer{}
	t.Cleanup(func() { log.Out = old })
}

// Every validation failure must surface before any file is written.
func TestRunRejectsInvalidChoices(t *testing.T) {
	quiet(t)
	t.Setenv("HOME", t.TempDir())
	base := Options{
		Path: t.TempDir(), Name: "x", Agent: "claude", IDE: "vscode",
		StacksProvided: true, FeaturesProvided: true,
		SecretProvider: "none", SecretProviderProvided: true,
	}
	tests := []struct {
		name string
		mut  func(*Options)
		want string
	}{
		{"agent", func(o *Options) { o.Agent = "gemini" }, "Invalid agent: gemini"},
		{"ide", func(o *Options) { o.IDE = "emacs" }, "Invalid IDE: emacs"},
		{"provider", func(o *Options) { o.SecretProvider = "vault" }, "Invalid secret provider: vault"},
		{"proxy", func(o *Options) { o.Proxy = "gui" }, "Invalid proxy mode: gui"},
		{"feature", func(o *Options) { o.Features = "tui,turbo" }, "Unknown feature: turbo"},
		{"feature 1password", func(o *Options) { o.Features = "1password" }, "Use --secret-provider 1password"},
		{"stack", func(o *Options) { o.Stacks = "node,cobol" }, "Invalid stack: cobol"},
		{"1password conflict", func(o *Options) { o.OnePasswordAlias = true }, "Do not combine --1password"},
		{"missing dir", func(o *Options) { o.Path = filepath.Join(o.Path, "nope") }, "Directory does not exist"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := base
			o.Path = t.TempDir() // per case, so one leak can't mask another
			tc.mut(&o)
			err := Run(o)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want error containing %q", err, tc.want)
			}
			if entries, _ := os.ReadDir(o.Path); len(entries) != 0 && tc.name != "missing dir" {
				t.Errorf("files were written despite the error: %v", entries)
			}
		})
	}
}

func TestManageGitignoreStatuses(t *testing.T) {
	quiet(t)
	plain := t.TempDir()
	if got := manageGitignore(plain, true); got != "skipped (no .git in project)" {
		t.Errorf("no .git: %q", got)
	}

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		enabled bool
		want    string
	}{
		{false, "skipped (disabled)"},
		{true, "added Sandcat block"},
		{true, "Sandcat block already present"},
		{false, "removed Sandcat block (disabled)"},
	}
	for _, s := range steps {
		if got := manageGitignore(repo, s.enabled); got != s.want {
			t.Errorf("enabled=%v: got %q, want %q", s.enabled, got, s.want)
		}
	}
	if gitignore.HasBlock(repo) {
		t.Error("block should be gone after the final disable")
	}
}

func TestEnvBool(t *testing.T) {
	os.Unsetenv("SANDCAT_TEST_TOGGLE")
	if !envBool("SANDCAT_TEST_TOGGLE", true) || envBool("SANDCAT_TEST_TOGGLE", false) {
		t.Error("unset should return the default")
	}
	t.Setenv("SANDCAT_TEST_TOGGLE", "false")
	if envBool("SANDCAT_TEST_TOGGLE", true) {
		t.Error("\"false\" should disable")
	}
	t.Setenv("SANDCAT_TEST_TOGGLE", "yes")
	if envBool("SANDCAT_TEST_TOGGLE", true) {
		t.Error("only the literal \"true\" enables")
	}
}
