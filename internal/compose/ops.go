package compose

import (
	"fmt"
	"path"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/stacks"
)

// SetProjectName sets the compose project name as the document's *first* key,
// matching `. = {"name": env(x)} * .` — which builds a new mapping with name
// leading and merges the rest in after it.
func (f *File) SetProjectName(name string) {
	root := f.root()
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "name" {
			root.Content[i+1].Value = name
			return
		}
	}
	root.Content = append([]*yaml.Node{scalar("name"), scalar(name)}, root.Content...)
}

// agentVolumes returns the services.agent.volumes sequence, creating it if
// the template ever drops it.
func (f *File) agentVolumes() *yaml.Node {
	return ensureSeq(f.root(), "services", "agent", "volumes")
}

// AddVolumeEntry appends a mount to the agent service.
//
// When active, the entry is a real list item carrying comment as a head
// comment. When not, nothing is added to the sequence — instead the entry is
// appended, commented out, to the foot comment of the last active entry, so
// the user can enable it by uncommenting. This asymmetry is the whole reason
// the port edits nodes rather than marshaling structs.
func (f *File) AddVolumeEntry(entry string, active bool, comment string) error {
	seq := f.agentVolumes()
	if active {
		item := scalar(entry)
		if comment != "" {
			item.HeadComment = comment
		}
		seq.Content = append(seq.Content, item)
		return nil
	}

	text := "- " + entry
	if comment != "" {
		text = comment + "\n" + text
	}
	return appendFootComment(seq, text)
}

// SetWorkspace sets the agent's working directory and mounts the project.
func (f *File) SetWorkspace(projectName string) error {
	workspace := "/workspaces/" + projectName
	f.setScalar(workspace, "services", "agent", "working_dir")

	for _, v := range []struct{ entry, comment string }{
		{"..:" + workspace, "Mount the project's code"},
		{"../.devcontainer:" + workspace + "/.devcontainer:ro", "Read-only devcontainer directory"},
		{"../.sandcat:" + workspace + "/.sandcat:ro", "Read-only settings directory"},
	} {
		if err := f.AddVolumeEntry(v.entry, true, v.comment); err != nil {
			return err
		}
	}
	return nil
}

// settingsVolumeComment explains why a missing .sandcat directory is not an
// error, since Docker will silently create one.
const settingsVolumeComment = `Project-level settings (.sandcat/ directory). If the directory does
not exist on the host, Docker creates an empty one and the addon
simply finds no files — no error.`

// AddSettingsVolume mounts the project settings directory into mitmproxy.
func (f *File) AddSettingsVolume(settingsFile string) error {
	seq := ensureSeq(f.root(), "services", "mitmproxy", "volumes")
	seq.Content = append(seq.Content, scalar(path.Dir(settingsFile)+":/config/project:ro"))
	return appendFootComment(seq, settingsVolumeComment)
}

// AddAgentConfigVolumes adds the optional host-config mounts for an agent.
//
// Ordering and comment placement match the per-agent helpers in
// composefile.bash: only the first entry carries the group heading.
func (f *File) AddAgentConfigVolumes(a agents.Agent, active bool, projectName string) error {
	var entries []string
	var heading string

	switch a.Name {
	case "claude":
		heading = "Host Claude config (optional)"
		entries = []string{
			"${HOME}/.claude/CLAUDE.md:/home/vscode/.claude/CLAUDE.md:ro",
			"${HOME}/.claude/agents:/home/vscode/.claude/agents:ro",
			"${HOME}/.claude/commands:/home/vscode/.claude/commands:ro",
		}
	case "codex":
		heading = "Host Codex config (optional) — copied into writable ~/.codex/AGENTS.md by app-user-init.sh so rtk can patch it"
		entries = []string{
			"${HOME}/.codex/AGENTS.md:/home/vscode/.codex-host/AGENTS.md:ro",
			"${HOME}/.codex/skills:/home/vscode/.codex/skills:ro",
			"${HOME}/.codex/commands:/home/vscode/.codex/commands:ro",
		}
	case "cursor":
		heading = "Host Cursor config (optional)"
		// projects/<id> is workspace-scoped and read-write; chats/, plugins/
		// and subagents/ deliberately stay in agent-home so this sandbox can't
		// see other workspaces' runtime state from the host profile.
		id := agents.CursorWorkspaceProjectID(projectName)
		entries = []string{
			"${HOME}/.cursor/AGENTS.md:/home/vscode/.cursor/AGENTS.md:ro",
			"${HOME}/.cursor/rules:/home/vscode/.cursor/rules:ro",
			"${HOME}/.cursor/skills:/home/vscode/.cursor/skills:ro",
			"${HOME}/.cursor/commands:/home/vscode/.cursor/commands:ro",
			"${HOME}/.cursor/hooks.json:/home/vscode/.cursor/hooks.json:ro",
			"${HOME}/.cursor/hooks:/home/vscode/.cursor/hooks:ro",
			"${HOME}/.cursor/agents:/home/vscode/.cursor/agents:ro",
			"${HOME}/.cursor/mcp.json:/home/vscode/.cursor/mcp.json:ro",
			"${HOME}/.cursor/projects/" + id + ":/home/vscode/.cursor/projects/" + id,
		}
	default:
		return nil
	}

	for i, e := range entries {
		comment := ""
		if i == 0 {
			comment = heading
		}
		if err := f.AddVolumeEntry(e, active, comment); err != nil {
			return err
		}
	}
	return nil
}

// AddGitReadonlyVolume mounts .git read-only.
func (f *File) AddGitReadonlyVolume(active bool) error {
	return f.AddVolumeEntry("../.git:/workspace/.git:ro", active, "Read-only Git directory")
}

// AddIdeaReadonlyVolume mounts .idea read-only.
func (f *File) AddIdeaReadonlyVolume(active bool) error {
	return f.AddVolumeEntry("../.idea:/workspace/.idea:ro", active, "Read-only IntelliJ IDEA project directory")
}

const sharedCacheComment = "Shared dependency caches for the selected stacks (SANDCAT_MOUNT_SHARED_CACHE=false to disable)"

// AddSharedCacheVolumes mounts the dependency caches contributed by the given
// resolved stacks, and declares each as an external volume.
//
// External is deliberate: the caches are host-scoped and shared across every
// sandbox, so `sandcat compose down -v` in one project must not wipe caches
// another project depends on. `sandcat run` creates them lazily.
//
// When inactive the mounts are added as comments and no volumes are declared —
// declaring an external volume nothing mounts would just fail at `up`.
func (f *File) AddSharedCacheVolumes(active bool, resolvedStacks []string) error {
	caches := stacks.SharedCaches(resolvedStacks)
	if len(caches) == 0 {
		return nil
	}

	for i, c := range caches {
		comment := ""
		if i == 0 {
			comment = sharedCacheComment
		}
		if err := f.AddVolumeEntry(c.String(), active, comment); err != nil {
			return err
		}
	}

	if !active {
		return nil
	}
	volumes := ensure(f.root(), "volumes")
	for _, c := range caches {
		decl := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			scalar("external"), {Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
			scalar("name"), scalar(c.Volume),
		}}
		appendContent(volumes, scalar(c.Volume), decl)
	}
	return nil
}

// jetbrainsCaps are the capabilities the JetBrains backend needs to manage its
// own cache and state files on mounted volumes.
var jetbrainsCaps = []struct{ cap, why string }{
	{"DAC_OVERRIDE", "JetBrains IDE: bypass file permission checks on mounted volumes"},
	{"CHOWN", "JetBrains IDE: change ownership of IDE cache and state files"},
	{"FOWNER", "JetBrains IDE: bypass ownership checks on IDE-managed files"},
}

// AddJetBrainsCapabilities grants the agent service the caps the JetBrains
// backend requires.
func (f *File) AddJetBrainsCapabilities() {
	seq := ensureSeq(f.root(), "services", "agent", "cap_add")
	for _, c := range jetbrainsCaps {
		item := scalar(c.cap)
		item.HeadComment = c.why
		seq.Content = append(seq.Content, item)
	}
}

// MitmproxyVersion pins the mitmproxy image used by generated compose files
// (SCT_MITMPROXY_VERSION in constants.bash). It must stay equal to
// MITMPROXY_VERSION in images/mitmproxy.env, which the image builds consume;
// a contract test asserts the two match.
const MitmproxyVersion = "12.2.3"

// SecretProvider selects the mitmproxy image and the token it reads.
type SecretProvider string

const (
	SecretProviderNone       SecretProvider = "none"
	SecretProvider1Password  SecretProvider = "1password"
	SecretProviderProtonPass SecretProvider = "protonpass"
)

// SecretProviders lists the valid values.
var SecretProviders = []SecretProvider{SecretProviderNone, SecretProvider1Password, SecretProviderProtonPass}

// ValidSecretProvider reports whether s names a known provider.
func ValidSecretProvider(s string) bool {
	for _, p := range SecretProviders {
		if string(p) == s {
			return true
		}
	}
	return false
}

// ApplySecretProvider points mitmproxy at the provider-specific image and
// passes through the credential it needs.
func (f *File) ApplySecretProvider(provider SecretProvider) error {
	var image, token string
	switch provider {
	case SecretProviderNone, "":
		return nil
	case SecretProvider1Password:
		image, token = "ghcr.io/virtuslab/sandcat-mitmproxy-op:"+MitmproxyVersion, "OP_SERVICE_ACCOUNT_TOKEN"
	case SecretProviderProtonPass:
		image, token = "ghcr.io/virtuslab/sandcat-mitmproxy-pass:"+MitmproxyVersion, "PROTON_PASS_PERSONAL_ACCESS_TOKEN"
	default:
		return fmt.Errorf("unknown secret provider: %s", provider)
	}

	f.setScalar(image, "services", "mitmproxy", "image")
	mitmproxy := ensure(f.root(), "services", "mitmproxy")
	env := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{scalar(token)}}
	replaced := false
	for i := 0; i+1 < len(mitmproxy.Content); i += 2 {
		if mitmproxy.Content[i].Value == "environment" {
			mitmproxy.Content[i+1] = env
			replaced = true
			break
		}
	}
	if !replaced {
		appendContent(mitmproxy, scalar("environment"), env)
	}
	return nil
}

var (
	reWebHost = regexp.MustCompile(`\s+--web-host\s+\S+`)
	reWebPass = regexp.MustCompile(`\s+--set\s+web_password=\S+`)
	reMitmweb = regexp.MustCompile(`^mitmweb\b`)
)

// SetProxyTUIMode switches mitmproxy from the web UI to console mode.
//
// mitmdump logs flows as text to stdout, so `sandcat proxy` tails the logs
// instead of opening a browser; the web-only flags and the published port
// have to go with it.
func (f *File) SetProxyTUIMode() {
	cmd := get(f.root(), "services", "mitmproxy", "command")
	if cmd != nil && cmd.Kind == yaml.ScalarNode {
		v := reMitmweb.ReplaceAllString(cmd.Value, "mitmdump")
		v = reWebHost.ReplaceAllString(v, "")
		v = reWebPass.ReplaceAllString(v, "")
		cmd.Value = v
	}
	f.delete("services", "mitmproxy", "ports")
}

// MergeAgentEnvironment appends KEY=value entries to services.agent.environment,
// keeping anything already there so agent- and stack-contributed variables
// coexist regardless of call order.
//
// A no-op with no entries: compose rejects an empty `environment: {}` block,
// which is why the bash builds the array conditionally rather than always
// emitting the key.
func (f *File) MergeAgentEnvironment(entries []string) {
	if len(entries) == 0 {
		return
	}
	seq := ensureSeq(f.root(), "services", "agent", "environment")
	for _, e := range entries {
		seq.Content = append(seq.Content, scalar(e))
	}
}
