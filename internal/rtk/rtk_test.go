package rtk

import (
	"strings"
	"testing"
)

func TestDockerInstallBlock(t *testing.T) {
	got := DockerInstallBlock(true)
	if !strings.Contains(got, "RTK_INSTALL_DIR=/usr/local/bin") {
		t.Errorf("missing the install invocation: %q", got)
	}
	// Installed system-wide on purpose: the agent-home volume would otherwise
	// mask a binary written under /home/vscode on upgrade.
	if !strings.Contains(got, "USER root") || !strings.Contains(got, "USER vscode") {
		t.Errorf("install block should switch to root and back: %q", got)
	}
	if DockerInstallBlock(false) != "" {
		t.Error("disabled should yield an empty block")
	}
}

func TestUserInitBlockPerAgent(t *testing.T) {
	tests := []struct {
		agent    string
		wantInit bool
	}{
		{"claude", true},
		{"cursor", true},
		// rtk 0.44 has no codex profile: --agent codex does not exist, and
		// --codex cannot combine with --hook-only or --auto-patch.
		{"codex", false},
		{"unknown", false},
	}
	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			got := UserInitBlock(tc.agent, true)
			if tc.wantInit && got == "" {
				t.Error("expected a block")
			}
			if !tc.wantInit && got != "" {
				t.Errorf("expected no block, got %q", got)
			}
		})
	}
}

func TestUserInitBlockDisabled(t *testing.T) {
	for _, agent := range []string{"claude", "cursor", "codex"} {
		if got := UserInitBlock(agent, false); got != "" {
			t.Errorf("%s: disabled should yield an empty block, got %q", agent, got)
		}
	}
}

// The init fragments are guarded so a restart doesn't re-patch config that
// sandcat mounts read-only.
func TestUserInitBlocksAreGuarded(t *testing.T) {
	for _, agent := range []string{"claude", "cursor"} {
		if !strings.Contains(UserInitBlock(agent, true), "command -v rtk") {
			t.Errorf("%s: init fragment should guard on rtk being present", agent)
		}
	}
}

func TestBlocksHaveNoTrailingNewline(t *testing.T) {
	for _, got := range []string{DockerInstallBlock(true), UserInitBlock("claude", true), UserInitBlock("cursor", true)} {
		if strings.HasSuffix(got, "\n") {
			t.Errorf("block ends with a newline: %q", got)
		}
	}
}
