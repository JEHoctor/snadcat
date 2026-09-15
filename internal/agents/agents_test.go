package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAvailableMatchesBashOrder(t *testing.T) {
	want := []string{"claude", "cursor", "codex"}
	if got := Available(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestIsValid(t *testing.T) {
	for _, n := range Available() {
		if !IsValid(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range []string{"", "copilot", "Claude"} {
		if IsValid(n) {
			t.Errorf("%q should not be valid", n)
		}
	}
}

func TestCursorWorkspaceProjectID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"demo-sandbox", "workspaces-demo-sandbox"},
		{"a/b", "workspaces-a-b"},
	}
	for _, tc := range tests {
		if got := CursorWorkspaceProjectID(tc.in); got != tc.want {
			t.Errorf("CursorWorkspaceProjectID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Only cursor needs mitmproxy's streaming flags. Setting them for claude would
// unbuffer response bodies and disable the addon's placeholder leak check, so
// an accidental change here is a security regression rather than a perf tweak.
func TestOnlyCursorSetsMitmStreamingFlags(t *testing.T) {
	for _, name := range Available() {
		a, _ := Get(name)
		if name == "cursor" {
			if a.MitmStreamingFlags == "" {
				t.Error("cursor must set streaming flags")
			}
			continue
		}
		if a.MitmStreamingFlags != "" {
			t.Errorf("%s must not set streaming flags, got %q", name, a.MitmStreamingFlags)
		}
	}
}

// compose rejects `environment: {}`, so callers key off emptiness here.
func TestComposeEnvironment(t *testing.T) {
	claude, _ := Get("claude")
	if want := []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"}; !reflect.DeepEqual(claude.ComposeEnvironment, want) {
		t.Errorf("got %v, want %v", claude.ComposeEnvironment, want)
	}
	for _, name := range []string{"cursor", "codex"} {
		a, _ := Get(name)
		if len(a.ComposeEnvironment) != 0 {
			t.Errorf("%s should contribute no environment, got %v", name, a.ComposeEnvironment)
		}
	}
}

// rtk's install block is agent-agnostic and must be appended to every agent's
// install block when enabled, and to none when disabled.
func TestDockerInstallBlockAppendsRTK(t *testing.T) {
	const marker = "RTK_INSTALL_DIR=/usr/local/bin"
	for _, name := range Available() {
		a, _ := Get(name)
		if !strings.Contains(a.DockerInstallBlock(true), marker) {
			t.Errorf("%s: rtk install block missing when enabled", name)
		}
		if strings.Contains(a.DockerInstallBlock(false), marker) {
			t.Errorf("%s: rtk install block present when disabled", name)
		}
	}
}

// rtk 0.44 has no codex profile, so codex gets no rtk user-init fragment — it
// wires rtk from its own block instead. claude and cursor do get one.
func TestUserInitBlockRTKWiring(t *testing.T) {
	claude, _ := Get("claude")
	if !strings.Contains(claude.UserInitBlock(true), "rtk init -g --hook-only --auto-patch") {
		t.Error("claude should get an rtk init fragment")
	}

	codex, _ := Get("codex")
	withRTK := codex.UserInitBlock(true)
	if withRTK != codex.UserInitBlock(false) {
		t.Error("codex user-init should not vary with the rtk toggle")
	}
}

// Blocks are captured with trailing newlines stripped, matching the $(...)
// capture the bash callers use; a stray blank line would shift every
// subsequent line of the generated Dockerfile.
func TestBlocksHaveNoTrailingNewline(t *testing.T) {
	for _, name := range Available() {
		a, _ := Get(name)
		for label, got := range map[string]string{
			"docker-install": a.DockerInstallBlock(true),
			"home-prep":      a.DockerHomePrepBlock(),
			"user-init":      a.UserInitBlock(true),
			"settings":       a.DevcontainerSettingsBlock(),
		} {
			if strings.HasSuffix(got, "\n") {
				t.Errorf("%s/%s ends with a newline", name, label)
			}
		}
	}
}

func TestHostConfigPaths(t *testing.T) {
	cursor, _ := Get("cursor")
	got := cursor.HostConfigPaths("demo-sandbox")

	var found bool
	for _, p := range got {
		if p.Path == ".cursor/projects/workspaces-demo-sandbox" {
			if !p.IsDir {
				t.Error("the workspace projects path should be a directory")
			}
			found = true
		}
	}
	if !found {
		t.Errorf("cursor paths missing the workspace-scoped projects dir: %v", got)
	}
}

func TestEnsureHostConfigPathsCreatesDirsAndSeedsJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SANDCAT_MOUNT_CURSOR_CONFIG", "true")

	cursor, _ := Get("cursor")
	if err := cursor.EnsureHostConfigPaths(home, "demo-sandbox"); err != nil {
		t.Fatal(err)
	}

	if fi, err := os.Stat(filepath.Join(home, ".cursor/rules")); err != nil || !fi.IsDir() {
		t.Errorf(".cursor/rules should be a directory: %v", err)
	}

	// Zero-length JSON breaks Cursor CLI's parser, so these get minimal
	// valid documents rather than empty files.
	for name, want := range map[string]string{
		"hooks.json": "{\"hooks\":{}}\n",
		"mcp.json":   "{\"mcpServers\":{}}\n",
	} {
		b, err := os.ReadFile(filepath.Join(home, ".cursor", name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if string(b) != want {
			t.Errorf("%s = %q, want %q", name, b, want)
		}
	}

	// AGENTS.md is Markdown; empty is valid and expected.
	if b, err := os.ReadFile(filepath.Join(home, ".cursor/AGENTS.md")); err != nil || len(b) != 0 {
		t.Errorf("AGENTS.md should exist and be empty, got %q err %v", b, err)
	}
}

// A user's real config must survive re-running init.
func TestEnsureHostConfigPathsDoesNotClobberExistingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SANDCAT_MOUNT_CURSOR_CONFIG", "true")

	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const existing = `{"mcpServers":{"mine":{}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cursor, _ := Get("cursor")
	if err := cursor.EnsureHostConfigPaths(home, "demo-sandbox"); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != existing {
		t.Errorf("existing config was overwritten: got %q, want %q", b, existing)
	}
}

func TestEnsureHostConfigPathsSkippedWhenMountDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SANDCAT_MOUNT_CURSOR_CONFIG", "false")

	cursor, _ := Get("cursor")
	if err := cursor.EnsureHostConfigPaths(home, "demo-sandbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor")); !os.IsNotExist(err) {
		t.Error("nothing should have been created when the mount is disabled")
	}
}

// Unset means enabled: customize_compose_file defaults the mounts to "true",
// and the pre-created paths must agree with the generated mounts.
func TestMountEnabledDefaultsOnWhenUnset(t *testing.T) {
	claude, _ := Get("claude")
	os.Unsetenv(claude.MountEnvVar)
	if !claude.MountEnabled() {
		t.Error("unset should mean enabled")
	}
	t.Setenv(claude.MountEnvVar, "anything-else")
	if claude.MountEnabled() {
		t.Error("only the literal \"true\" should enable the mount")
	}
}

// bashBlock runs one of the original dispatchers so the Go table can be
// checked against the source of truth rather than against a transcription.
func bashBlock(t *testing.T, env []string, fn string, args ...string) (string, bool) {
	t.Helper()
	libdir, err := filepath.Abs("../../cli/lib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(libdir, "agents.bash")); err != nil {
		return "", false // bash tree already removed (post-cutover)
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "", false
	}

	script := "set -euo pipefail; source \"$SCT_LIBDIR/agents.bash\"; source \"$SCT_LIBDIR/rtk.bash\"; " +
		fn + " \"$@\""
	cmd := exec.Command("bash", append([]string{"-c", script, "bash"}, args...)...)
	cmd.Env = append(append(os.Environ(), "SCT_LIBDIR="+libdir), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bash %s %v: %v", fn, args, err)
	}
	return strings.TrimRight(string(out), "\n"), true
}

// The Go table must produce byte-identical output to the bash dispatchers it
// replaces — this is the parity check the whole port rests on.
func TestBlocksMatchBashOutput(t *testing.T) {
	for _, name := range Available() {
		a, _ := Get(name)
		t.Run(name, func(t *testing.T) {
			cases := []struct {
				label string
				env   []string
				fn    string
				got   string
			}{
				{"docker-install+rtk", []string{"SANDCAT_RTK=true"}, "sct_agent_docker_install_block", a.DockerInstallBlock(true)},
				{"docker-install-no-rtk", []string{"SANDCAT_RTK=false"}, "sct_agent_docker_install_block", a.DockerInstallBlock(false)},
				{"user-init+rtk", []string{"SANDCAT_RTK=true"}, "sct_agent_user_init_block", a.UserInitBlock(true)},
				{"user-init-no-rtk", []string{"SANDCAT_RTK=false"}, "sct_agent_user_init_block", a.UserInitBlock(false)},
				{"home-prep", nil, "sct_agent_docker_home_prep_block", a.DockerHomePrepBlock()},
				{"devcontainer-settings", nil, "sct_agent_devcontainer_settings_block", a.DevcontainerSettingsBlock()},
				{"vscode-extension", nil, "sct_agent_vscode_extension", a.VSCodeExtension},
				{"mount-env-var", nil, "sct_agent_mount_env_var", a.MountEnvVar},
				{"api-key-help", nil, "sct_agent_api_key_help", a.APIKeyHelp},
				{"op-api-key-help", nil, "sct_agent_op_api_key_help", a.OpAPIKeyHelp},
				{"mitm-streaming-flags", nil, "sct_agent_mitm_streaming_flags", a.MitmStreamingFlags},
			}
			for _, tc := range cases {
				want, ok := bashBlock(t, tc.env, tc.fn, name)
				if !ok {
					t.Skip("bash originals unavailable")
				}
				if tc.got != want {
					t.Errorf("%s:\n got: %q\nwant: %q", tc.label, tc.got, want)
				}
			}
		})
	}
}

func TestHostConfigPathsMatchBashOutput(t *testing.T) {
	const project = "demo-sandbox"
	for _, name := range Available() {
		a, _ := Get(name)
		t.Run(name, func(t *testing.T) {
			out, ok := bashBlock(t, nil, "sct_agent_host_config_paths", name, project)
			if !ok {
				t.Skip("bash originals unavailable")
			}

			var want []HostPath
			for _, line := range strings.Split(out, "\n") {
				if line == "" {
					continue
				}
				p := strings.TrimPrefix(line, "$HOME/")
				want = append(want, HostPath{Path: strings.TrimSuffix(p, "/"), IsDir: strings.HasSuffix(p, "/")})
			}
			if got := a.HostConfigPaths(project); !reflect.DeepEqual(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}
