package devcontainer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	snadcat "github.com/jehoctor/snadcat"
	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/compose"
	"github.com/jehoctor/snadcat/internal/devbox"
	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/stacks"
)

// Options configure one generation of a project's .devcontainer directory.
type Options struct {
	ProjectPath string
	ProjectName string

	// SettingsFile is the project settings path relative to ProjectPath,
	// e.g. ".sandcat/settings.json".
	SettingsFile string

	// UserSettings is the absolute path of ~/.config/snadcat/settings.json,
	// read for upstream_ca_bundles. Empty means none configured there.
	UserSettings string

	Agent agents.Agent
	IDE   string

	// Stacks are resolved stack names.
	Stacks []string

	ProxyTUI       bool
	SecretProvider compose.SecretProvider
	RTKEnabled     bool

	// Mounts are the compose mount toggles, already defaulted and
	// env-overridden by the caller.
	Mounts compose.Options
}

// Generate writes ProjectPath/.devcontainer from the embedded templates.
//
// The step order is the bash order in libexec/init/devcontainer, and it is
// not free to change: __DEVBOX_INSTALL__ must expand before the agent
// placeholders in the same Dockerfile, and services.agent.environment must
// land in compose-all.yml before Customize appends working_dir, or keys come
// out in a different order.
func Generate(o Options) error {
	dir := filepath.Join(o.ProjectPath, ".devcontainer")
	if err := copyTemplates(dir); err != nil {
		return err
	}

	// devcontainer.json: stack extensions first; IDE block and project name
	// come last, after the agent placeholders.
	jsonPath := filepath.Join(dir, "devcontainer.json")
	if err := editFile(jsonPath, func(s string) string {
		return ApplyLinePlaceholders(s, Pair{"__STACK_EXTENSIONS__", StackExtensionLines(o.Stacks)})
	}); err != nil {
		return err
	}

	// devbox: regenerate the stack file, seed the tools file once, expand the
	// Dockerfile block.
	if err := devbox.WriteStackJSON(filepath.Join(dir, "devbox.stack.json"), o.Stacks); err != nil {
		return err
	}
	if err := devbox.WriteToolsJSON(filepath.Join(dir, "devbox.tools.json")); err != nil {
		return err
	}
	dockerfile := filepath.Join(dir, "Dockerfile.app")
	if err := editFile(dockerfile, func(s string) string {
		return ApplyLinePlaceholders(s, Pair{"__DEVBOX_INSTALL__", devbox.DockerfileBlock()})
	}); err != nil {
		return err
	}

	// Stack-contributed environment lands before the agent's, matching the
	// bash call order (customize_compose_stack_environment runs first).
	composePath := filepath.Join(dir, "compose-all.yml")
	cf, err := compose.Load(composePath)
	if err != nil {
		return err
	}
	cf.MergeAgentEnvironment(stacks.EnvEntries(o.Stacks))

	// Agent-specific placeholders across four files.
	a := o.Agent
	if err := editFile(jsonPath, func(s string) string {
		return ApplyLinePlaceholders(s,
			Pair{"__AGENT_EXTENSION__", AgentExtensionLine(a.VSCodeExtension)},
			Pair{"__AGENT_SETTINGS__", a.DevcontainerSettingsBlock()},
		)
	}); err != nil {
		return err
	}
	if err := editFile(dockerfile, func(s string) string {
		return ApplyLinePlaceholders(s,
			Pair{"__AGENT_DOCKER_INSTALL__", a.DockerInstallBlock(o.RTKEnabled)},
			Pair{"__AGENT_DOCKER_HOME_PREP__", a.DockerHomePrepBlock()},
		)
	}); err != nil {
		return err
	}
	if err := editFile(filepath.Join(dir, "sandcat", "scripts", "app-user-init.sh"), func(s string) string {
		return ApplyLinePlaceholders(s, Pair{"__AGENT_USER_INIT__", a.UserInitBlock(o.RTKEnabled)})
	}); err != nil {
		return err
	}
	proxyPath := filepath.Join(dir, "sandcat", "compose-proxy.yml")
	if err := editFile(proxyPath, func(s string) string {
		return ApplyInlinePlaceholders(s,
			Pair{"__AGENT_MITM_ADDON__", a.MitmAddonFile},
			Pair{"__MITM_HTTP2__", a.MitmHTTP2},
			Pair{"__AGENT_MITM_STREAMING_FLAGS__", a.MitmStreamingFlags},
			Pair{"__MITMPROXY_VERSION__", compose.MitmproxyVersion},
		)
	}); err != nil {
		return err
	}

	// compose-proxy.yml structural edits.
	bundles := compose.ReadUpstreamCABundles(o.UserSettings, o.ProjectPath)
	if o.ProxyTUI || (o.SecretProvider != "" && o.SecretProvider != compose.SecretProviderNone) || len(bundles) > 0 {
		proxy, err := compose.Load(proxyPath)
		if err != nil {
			return err
		}
		if o.ProxyTUI {
			proxy.SetProxyTUIMode()
		}
		if err := proxy.ApplySecretProvider(o.SecretProvider); err != nil {
			return err
		}
		if err := proxy.ApplyUpstreamCABundles(bundles); err != nil {
			return err
		}
		if err := proxy.Save(proxyPath); err != nil {
			return err
		}
	}

	// compose-all.yml: agent environment, mounts, project name.
	cf.MergeAgentEnvironment(a.ComposeEnvironment)
	mounts := o.Mounts
	mounts.SettingsFile = "../" + filepath.ToSlash(o.SettingsFile)
	mounts.Agent = a
	mounts.IDE = o.IDE
	mounts.ProjectName = o.ProjectName
	mounts.Stacks = o.Stacks
	if err := cf.Customize(dir, mounts); err != nil {
		return err
	}
	cf.SetProjectName(o.ProjectName)
	if err := cf.Save(composePath); err != nil {
		return err
	}

	if err := editFile(jsonPath, func(s string) string {
		return CustomizeJSON(s, o.ProjectName, o.IDE)
	}); err != nil {
		return err
	}
	// Plugins go in after CustomizeJSON has emitted the JetBrains block.
	if o.IDE == "jetbrains" && len(o.Stacks) > 0 {
		if err := editFile(jsonPath, func(s string) string {
			return CustomizePlugins(s, o.Stacks)
		}); err != nil {
			return err
		}
	}

	log.Info("Devcontainer dir created at %s", ".devcontainer")
	return nil
}

// copyTemplates materializes the embedded devcontainer tree into dir,
// overwriting files that already exist. Executable bits are set on scripts
// since embed does not carry modes.
func copyTemplates(dir string) error {
	sub, err := fs.Sub(snadcat.Templates, "devcontainer")
	if err != nil {
		return err
	}
	return fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", p, err)
		}
		mode := os.FileMode(0o644)
		if executableTemplates[p] {
			mode = 0o755
		}
		return os.WriteFile(target, b, mode)
	})
}

// executableTemplates are the files git tracks as mode 100755. The rest of
// the scripts are invoked as `bash <file>` or sourced, so they are not.
var executableTemplates = map[string]bool{
	"sandcat/scripts/app-post-start.sh": true,
	"sandcat/scripts/dnsmasq-ready":     true,
}
