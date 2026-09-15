// Package agents ports cli/lib/agents.bash — the per-agent table driving
// everything that differs between claude, cursor and codex.
//
// The bash original is a set of parallel `case` dispatchers, one per property.
// Here they collapse into a single table, so adding an agent means adding one
// entry rather than editing nine functions and hoping none were missed.
//
// The multi-line Dockerfile / shell / JSONC fragments live in blocks/ as files
// rather than string literals; see scripts/dump-bash-blocks.sh for why and for
// how to regenerate them.
package agents

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/jehoctor/snadcat/internal/rtk"
)

//go:embed blocks
var blocksFS embed.FS

// HostPath is a host config location that an optional bind mount will target.
// Path is relative to the user's home directory.
type HostPath struct {
	Path  string
	IsDir bool
}

// Agent is one supported coding agent.
type Agent struct {
	Name string

	// MountEnvVar gates the optional host-config bind mounts for this agent.
	MountEnvVar string

	// VSCodeExtension is the extension id installed in the dev container.
	VSCodeExtension string

	// APIKeyHelp and OpAPIKeyHelp are the one-line hints printed by init's
	// "next steps" section, for plain values and 1Password references.
	APIKeyHelp   string
	OpAPIKeyHelp string

	// ComposeEnvironment are services.agent environment entries. Callers must
	// omit the YAML `environment:` key entirely when this is empty — compose
	// rejects an empty mapping.
	ComposeEnvironment []string

	// MitmAddonFile is the mitmproxy addon script for this agent's API shape.
	MitmAddonFile string

	// MitmHTTP2 is the literal substituted for __MITM_HTTP2__.
	MitmHTTP2 string

	// MitmStreamingFlags are extra mitmproxy --set flags. Empty for claude on
	// purpose: claude's traffic is plain buffered JSON, which lets the addon
	// run a content-based placeholder leak check that these flags would
	// defeat. Only cursor's Connect/HTTP-2 streaming needs them.
	MitmStreamingFlags string

	// hostPaths yields the host config locations to pre-create. It is a
	// function because cursor's set depends on the project name.
	hostPaths func(projectName string) []HostPath
}

var all = []Agent{
	{
		Name:               "claude",
		MountEnvVar:        "SANDCAT_MOUNT_CLAUDE_CONFIG",
		VSCodeExtension:    "anthropic.claude-code",
		APIKeyHelp:         "ANTHROPIC_API_KEY  your Anthropic API key (for Claude Code)",
		OpAPIKeyHelp:       `ANTHROPIC_API_KEY  "op": "op://vault/Anthropic API Key/credential"`,
		ComposeEnvironment: []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"},
		MitmAddonFile:      "mitmproxy_addon_claude.py",
		MitmHTTP2:          "true",
		hostPaths: func(string) []HostPath {
			return []HostPath{
				{".claude/agents", true},
				{".claude/commands", true},
				{".claude/CLAUDE.md", false},
			}
		},
	},
	{
		Name:               "cursor",
		MountEnvVar:        "SANDCAT_MOUNT_CURSOR_CONFIG",
		VSCodeExtension:    "anysphere.cursor",
		APIKeyHelp:         "CURSOR_API_KEY     your Cursor API key (for Cursor CLI)",
		OpAPIKeyHelp:       `CURSOR_API_KEY     "op": "op://vault/Cursor API Key/credential"`,
		MitmAddonFile:      "mitmproxy_addon_cursor.py",
		MitmHTTP2:          "true",
		MitmStreamingFlags: "--set stream_large_bodies=1m --set connection_strategy=lazy --set anticomp=true --set timeout_read=300",
		hostPaths: func(projectName string) []HostPath {
			return []HostPath{
				{".cursor/rules", true},
				{".cursor/skills", true},
				{".cursor/commands", true},
				{".cursor/agents", true},
				{".cursor/hooks", true},
				{".cursor/projects/" + CursorWorkspaceProjectID(projectName), true},
				{".cursor/AGENTS.md", false},
				{".cursor/hooks.json", false},
				{".cursor/mcp.json", false},
			}
		},
	},
	{
		Name:            "codex",
		MountEnvVar:     "SANDCAT_MOUNT_CODEX_CONFIG",
		VSCodeExtension: "openai.chatgpt",
		APIKeyHelp:      "OPENAI_API_KEY     your OpenAI API key (for Codex CLI)",
		OpAPIKeyHelp:    `OPENAI_API_KEY     "op": "op://vault/OpenAI API Key/credential"`,
		MitmAddonFile:   "mitmproxy_addon_codex.py",
		MitmHTTP2:       "true",
		hostPaths: func(string) []HostPath {
			return []HostPath{
				{".codex/AGENTS.md", false},
				{".codex/skills", true},
				{".codex/commands", true},
			}
		},
	},
	copilot,
}

var copilot = Agent{
	Name:            "copilot",
	MountEnvVar:     "SANDCAT_MOUNT_COPILOT_CONFIG",
	VSCodeExtension: "GitHub.copilot",
	APIKeyHelp:      `COPILOT_GITHUB_TOKEN  fine-grained GitHub PAT with "Copilot Requests" permission (or $(gh auth token))`,
	OpAPIKeyHelp:    `COPILOT_GITHUB_TOKEN  "op": "op://vault/GitHub Copilot Token/credential"`,
	MitmAddonFile:   "mitmproxy_addon_copilot.py",
	MitmHTTP2:       "true",
	hostPaths: func(string) []HostPath {
		return []HostPath{
			{".copilot/mcp-config.json", false},
			{".copilot/session-state", true},
		}
	},
}

var byName = func() map[string]Agent {
	m := make(map[string]Agent, len(all))
	for _, a := range all {
		m[a.Name] = a
	}
	return m
}()

// Available returns the supported agent names in presentation order — the
// order the interactive picker shows, so it is user-visible.
func Available() []string {
	out := make([]string, len(all))
	for i, a := range all {
		out[i] = a.Name
	}
	return out
}

// IsValid reports whether name is a supported agent.
func IsValid(name string) bool {
	_, ok := byName[name]
	return ok
}

// Get looks up an agent by name.
func Get(name string) (Agent, bool) {
	a, ok := byName[name]
	return a, ok
}

// FallbackAPIKeyHelp is printed for an agent with no specific hint. Reachable
// only if a caller bypasses validation, but the bash original has the arm so
// the port keeps it.
const FallbackAPIKeyHelp = "ANTHROPIC_API_KEY  API key for your selected agent"

// CursorWorkspaceProjectID maps a sandcat project name to Cursor's
// ~/.cursor/projects/<id> directory name. Cursor encodes the workspace path
// /workspaces/<name> by dropping the leading slash and replacing separators
// with hyphens, e.g. "workspaces-foo-bar".
func CursorWorkspaceProjectID(projectName string) string {
	return strings.ReplaceAll(strings.TrimPrefix("/workspaces/"+projectName, "/"), "/", "-")
}

// HostConfigPaths returns the host locations that this agent's optional config
// mounts will bind, relative to the user's home directory.
//
// They are pre-created because Docker otherwise materializes a missing bind
// source as a root-owned empty directory in the user's home — confusing and
// annoying to clean up.
func (a Agent) HostConfigPaths(projectName string) []HostPath {
	if a.hostPaths == nil {
		return nil
	}
	return a.hostPaths(projectName)
}

// DevcontainerSettingsBlock is the JSONC fragment spliced into
// devcontainer.json's settings object. Empty for agents that force no settings.
func (a Agent) DevcontainerSettingsBlock() string {
	return block("devcontainer-settings-" + a.Name)
}

// DockerHomePrepBlock pre-creates the agent's config directories in the image,
// so optional host mounts don't land as root-owned.
func (a Agent) DockerHomePrepBlock() string {
	return block("home-prep-" + a.Name)
}

// DockerInstallBlock is the Dockerfile.app fragment installing the agent CLI,
// followed by rtk's own install block when enabled.
func (a Agent) DockerInstallBlock(rtkEnabled bool) string {
	return join(block("docker-install-"+a.Name), rtk.DockerInstallBlock(rtkEnabled))
}

// UserInitBlock is the app-user-init.sh fragment bootstrapping the agent at
// container start, followed by rtk's init fragment when enabled.
func (a Agent) UserInitBlock(rtkEnabled bool) string {
	return join(block("user-init-"+a.Name), rtk.UserInitBlock(a.Name, rtkEnabled))
}

// join concatenates two fragments with a single newline, tolerating either
// being empty. The bash equivalent is one function `cat`-ing a heredoc and
// then calling the rtk helper, with the whole thing captured via $(...).
func join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

// block reads an embedded fragment, returning "" when absent. Trailing
// newlines are stripped to match the $(...) capture the bash callers use.
func block(name string) string {
	b, err := fs.ReadFile(blocksFS, "blocks/"+name+".txt")
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}
