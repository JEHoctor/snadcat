package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Parity with the user-settings functions in cli/libexec/init/init. HOME is
// pinned to a temp dir on both sides, which also empties the global git
// config so the identity falls back to the documented placeholders.

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("../..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func requireBashTooling(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(repoPath(t, "cli/libexec/init/init")); err != nil {
		t.Skip("bash originals unavailable")
	}
	for _, bin := range []string{"bash", "yq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s unavailable", bin)
		}
	}
}

// bashUserSettings sources init (its main guard keeps it from running), runs
// the given function calls under a fresh HOME, and returns the settings file.
func bashUserSettings(t *testing.T, seed string, calls string) string {
	t.Helper()
	home := t.TempDir()
	if seed != "" {
		dir := filepath.Join(home, ".config", "sandcat")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := `set -euo pipefail; source "$SCT_LIBEXECDIR/init/init"; ` + calls +
		`; cat "$HOME/.config/sandcat/settings.json"`
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"SCT_LIBDIR=" + repoPath(t, "cli/lib"),
		"SCT_LIBEXECDIR=" + repoPath(t, "cli/libexec"),
		"SCT_TEMPLATEDIR=" + repoPath(t, "cli/templates"),
		"GIT_CONFIG_NOSYSTEM=1",
	}
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("bash failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	return string(out)
}

// goUserSettings runs the Go equivalents under a fresh HOME.
func goUserSettings(t *testing.T, seed string, run func()) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	old := gitConfig
	gitConfig = func(string) string { return "" }
	t.Cleanup(func() { gitConfig = old })

	if seed != "" {
		dir := filepath.Join(home, ".config", "sandcat")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run()
	b, err := os.ReadFile(filepath.Join(home, ".config", "sandcat", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateUserSettingsMatchesBash(t *testing.T) {
	requireBashTooling(t)
	for _, agent := range []string{"claude", "cursor", "codex"} {
		t.Run(agent, func(t *testing.T) {
			want := bashUserSettings(t, "",
				`create_user_settings `+agent+`; sct_agent_post_user_settings_hook `+agent)
			got := goUserSettings(t, "", func() {
				must(t, CreateUserSettings(agent))
				must(t, ApplyAgentDefaults(agent))
			})
			if got != want {
				t.Errorf("differs from bash\n--- bash ---\n%s\n--- go ---\n%s", want, got)
			}
		})
	}
}

// Re-applying defaults to a file the user has already customised must keep
// their values, their key order, and not duplicate hosts or rules.
func TestAgentDefaultsPreserveUserEditsLikeBash(t *testing.T) {
	requireBashTooling(t)

	tests := []struct {
		name, agent, seed, bashFn string
	}{
		{
			name:  "cursor with existing hosts and http1 false",
			agent: "cursor",
			seed: `{"secrets":{"CURSOR_API_KEY":{"value":"k","hosts":["api2.cursor.sh","mine.example"]}},` +
				`"cursor":{"cli":{"version":2,"network":{"useHttp1ForAgent":false}}},"env":{"A":"1"}}` + "\n",
			bashFn: "ensure_cursor_user_settings_defaults",
		},
		{
			name:   "codex with existing openai rule",
			agent:  "codex",
			seed:   `{"network":[{"action":"allow","host":"api.openai.com"},{"action":"deny","host":"x"}],"secrets":{}}` + "\n",
			bashFn: "ensure_codex_user_settings_defaults",
		},
		{
			name:   "codex with method-qualified rule keeps both",
			agent:  "codex",
			seed:   `{"network":[{"action":"allow","host":"api.openai.com","method":"GET"}]}` + "\n",
			bashFn: "ensure_codex_user_settings_defaults",
		},
		{
			name:   "cursor from empty object",
			agent:  "cursor",
			seed:   "{}\n",
			bashFn: "ensure_cursor_user_settings_defaults",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := bashUserSettings(t, tc.seed, tc.bashFn)
			got := goUserSettings(t, tc.seed, func() { must(t, ApplyAgentDefaults(tc.agent)) })
			if got != want {
				t.Errorf("differs from bash\n--- bash ---\n%s\n--- go ---\n%s", want, got)
			}
		})
	}
}

func TestSecretProviderTokenMatchesBash(t *testing.T) {
	requireBashTooling(t)
	seed := `{"env":{},"op_service_account_token":"keep"}` + "\n"
	for _, provider := range []string{"1password", "protonpass", "none"} {
		t.Run(provider, func(t *testing.T) {
			want := bashUserSettings(t, seed, "add_secret_provider_tokens_to_user_settings "+provider)
			got := goUserSettings(t, seed, func() { must(t, AddSecretProviderToken(provider)) })
			if got != want {
				t.Errorf("differs from bash\n--- bash ---\n%s\n--- go ---\n%s", want, got)
			}
		})
	}
}

func TestConfiguredSecretProvider(t *testing.T) {
	tests := []struct{ seed, want string }{
		{`{"op_service_account_token":"ops_x"}`, "1password"},
		{`{"proton_pass_token":"pst_x"}`, "protonpass"},
		{`{"op_service_account_token":"","proton_pass_token":"pst_x"}`, "protonpass"},
		{`{"op_service_account_token":""}`, ""},
		{``, ""},
	}
	for _, tc := range tests {
		got := ""
		goUserSettingsNoRead(t, tc.seed, func() { got = ConfiguredSecretProvider() })
		if got != tc.want {
			t.Errorf("seed %q: got %q, want %q", tc.seed, got, tc.want)
		}
	}
}

func goUserSettingsNoRead(t *testing.T, seed string, run func()) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if seed != "" {
		dir := filepath.Join(home, ".config", "sandcat")
		must(t, os.MkdirAll(dir, 0o755))
		must(t, os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o644))
	}
	run()
}

func TestWriteProjectSettingsCopiesTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".sandcat", "settings.json")
	must(t, WriteProjectSettings(path))
	got, err := os.ReadFile(path)
	must(t, err)
	want, err := os.ReadFile(repoPath(t, "cli/templates/settings.json"))
	must(t, err)
	if string(got) != string(want) {
		t.Error("project settings differ from the template")
	}
}
