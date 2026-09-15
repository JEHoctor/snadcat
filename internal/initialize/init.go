// Package initialize ports cli/libexec/init/init — the `sandcat init` flow:
// resolve every choice from flags or prompts, seed user settings, write the
// project settings and .devcontainer tree, manage the .gitignore block, and
// print the next-steps summary.
package initialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/compose"
	"github.com/jehoctor/snadcat/internal/config"
	"github.com/jehoctor/snadcat/internal/devcontainer"
	"github.com/jehoctor/snadcat/internal/gitignore"
	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/project"
	"github.com/jehoctor/snadcat/internal/prompt"
	"github.com/jehoctor/snadcat/internal/stacks"
)

// Options are the `sandcat init` flags. A zero value for a string field means
// "prompt for it"; the *Provided booleans distinguish an explicitly empty flag
// (e.g. `--stacks ""` meaning no stacks) from an absent one.
type Options struct {
	Name           string
	Path           string
	Agent          string
	IDE            string
	Stacks         string
	Proxy          string
	SecretProvider string
	Features       string

	StacksProvided         bool
	FeaturesProvided       bool
	SecretProviderProvided bool
	OnePasswordAlias       bool
}

var ides = []string{"vscode", "jetbrains", "none"}

// featureLabels are the interactive picker's entries; the keyword before the
// first space is what the selection maps back to.
var featureLabels = []string{
	"tui (mitmproxy console instead of web UI)",
	"no-shared-cache (per-project dep cache instead of shared)",
	"no-gitignore (do not append Sandcat block to .gitignore)",
	"no-rtk (do not install rtk shell hook)",
	"strict-network (stack presets instead of allow-all-GET wildcard)",
}

// envBool reads a SANDCAT_* toggle: unset means def, otherwise only the
// literal "true" enables.
func envBool(name string, def bool) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	return v == "true"
}

// Run performs the initialization.
func Run(o Options) error {
	if o.OnePasswordAlias && o.SecretProviderProvided {
		return fmt.Errorf("Do not combine --1password with --secret-provider")
	}
	if o.OnePasswordAlias {
		o.SecretProvider = "1password"
		o.SecretProviderProvided = true
	}

	if o.Path == "" {
		o.Path = "."
	}
	if fi, err := os.Stat(o.Path); err != nil || !fi.IsDir() {
		return fmt.Errorf("Directory does not exist: %s", o.Path)
	}
	projectPath, err := filepath.Abs(o.Path)
	if err != nil {
		return err
	}
	log.Info("Configuring project at: %s", projectPath)

	// Name is asked before agent, and only defaulted after — matching the
	// prompt order users see.
	defaultName := project.DeriveName(projectPath)
	name := o.Name
	if name == "" {
		if name, err = prompt.Line(fmt.Sprintf("Project name [%s]:", defaultName)); err != nil {
			return err
		}
	}

	agentName := o.Agent
	if agentName == "" {
		if agentName, err = prompt.Option("Select agent:", agents.Available()); err != nil {
			return err
		}
	} else if !agents.IsValid(agentName) {
		return fmt.Errorf("Invalid agent: %s (expected: %s)", agentName, strings.Join(agents.Available(), " "))
	}
	agent, _ := agents.Get(agentName)

	if name == "" {
		name = defaultName
	}

	ide := o.IDE
	if ide == "" {
		if ide, err = prompt.Option("Select IDE:", ides); err != nil {
			return err
		}
	} else if !contains(ides, ide) {
		return fmt.Errorf("Invalid IDE: %s (expected: %s)", ide, strings.Join(ides, " "))
	}

	provider := o.SecretProvider
	if !o.SecretProviderProvided {
		// Put whichever backend already has a token first.
		var order []string
		switch config.ConfiguredSecretProvider() {
		case "1password":
			order = []string{"1password", "none", "protonpass"}
		case "protonpass":
			order = []string{"protonpass", "none", "1password"}
		default:
			order = []string{"none", "1password", "protonpass"}
		}
		if provider, err = prompt.Option("Select secret provider:", order); err != nil {
			return err
		}
	}
	if !compose.ValidSecretProvider(provider) {
		return fmt.Errorf("Invalid secret provider: %s (expected: none 1password protonpass)", provider)
	}

	// Optional features.
	proxyMode := o.Proxy
	gitignoreEnabled := envBool("SANDCAT_GITIGNORE", true)
	rtkEnabled := envBool("SANDCAT_RTK", true)
	strictNetwork := envBool("SANDCAT_STRICT_NETWORK", false)
	sharedCacheDisabled := false
	applyFeature := func(f string) error {
		switch f {
		case "tui":
			proxyMode = "tui"
		case "no-shared-cache":
			sharedCacheDisabled = true
		case "no-gitignore":
			gitignoreEnabled = false
		case "no-rtk":
			rtkEnabled = false
		case "strict-network":
			strictNetwork = true
		case "1password":
			return fmt.Errorf("Use --secret-provider 1password instead of --features 1password")
		default:
			return fmt.Errorf("Unknown feature: %s (expected: tui, no-shared-cache, no-gitignore, no-rtk, strict-network)", f)
		}
		return nil
	}
	if !o.FeaturesProvided {
		selected, err := prompt.Multiple("Select optional features (comma-separated numbers, empty for none):", featureLabels, nil)
		if err != nil {
			return err
		}
		for _, label := range selected {
			// The picker returns full labels; unknown words in a label are
			// ignored, as the bash's `for f in $selected_features` was.
			_ = applyFeature(strings.Fields(label)[0])
		}
	} else if o.Features != "" {
		for _, f := range strings.Split(o.Features, ",") {
			if err := applyFeature(f); err != nil {
				return err
			}
		}
	}
	if proxyMode != "" && proxyMode != "web" && proxyMode != "tui" {
		return fmt.Errorf("Invalid proxy mode: %s (expected: web, tui)", proxyMode)
	}
	if proxyMode == "" {
		proxyMode = "web"
	}

	// Stacks.
	var resolved []string
	if o.StacksProvided {
		if o.Stacks != "" {
			list := strings.Split(o.Stacks, ",")
			if err := stacks.Validate(list); err != nil {
				return err
			}
			resolved = stacks.Resolve(list)
		}
	} else {
		selected, err := prompt.Multiple("Select development stacks (comma-separated numbers, empty for none):", stacks.Names(), nil)
		if err != nil {
			return err
		}
		if len(selected) > 0 {
			resolved = stacks.Resolve(selected)
		}
	}

	// User-level state.
	if err := config.CreateUserSettings(agentName); err != nil {
		return err
	}
	if err := config.ApplyAgentDefaults(agentName); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := agent.EnsureHostConfigPaths(home, name); err != nil {
		return err
	}
	if err := config.AddSecretProviderToken(provider); err != nil {
		return err
	}

	// Project-level files.
	settingsRel := filepath.Join(project.Dir, "settings.json")
	if err := config.WriteProjectSettings(filepath.Join(projectPath, settingsRel), config.ProjectSettingsOptions{
		StrictNetwork: strictNetwork,
		Stacks:        resolved,
	}); err != nil {
		return err
	}

	mounts := compose.DefaultOptions()
	mounts.Agent = agent
	mounts.ApplyEnvOverrides(os.LookupEnv)
	if sharedCacheDisabled {
		// The feature wins over the environment, as the bash's export did.
		mounts.MountSharedCache = false
	}
	if err := devcontainer.Generate(devcontainer.Options{
		ProjectPath:    projectPath,
		ProjectName:    name,
		SettingsFile:   settingsRel,
		Agent:          agent,
		IDE:            ide,
		Stacks:         resolved,
		ProxyTUI:       proxyMode == "tui",
		SecretProvider: compose.SecretProvider(provider),
		RTKEnabled:     rtkEnabled,
		Mounts:         mounts,
	}); err != nil {
		return err
	}

	gitignoreStatus := manageGitignore(projectPath, gitignoreEnabled)

	printSummary(summary{
		agent:           agent,
		stacks:          resolved,
		gitignoreStatus: gitignoreStatus,
		rtkEnabled:      rtkEnabled,
		strictNetwork:   strictNetwork,
		provider:        provider,
	})
	return nil
}

// manageGitignore converges the .gitignore block to the requested state and
// reports what happened for the summary.
func manageGitignore(projectPath string, enabled bool) string {
	if _, err := os.Lstat(filepath.Join(projectPath, ".git")); err != nil {
		return "skipped (no .git in project)"
	}
	had := gitignore.HasBlock(projectPath)
	if enabled {
		if err := gitignore.Update(projectPath); err != nil {
			log.Warn("could not update .gitignore: %v", err)
			return "failed"
		}
		if had {
			return "Sandcat block already present"
		}
		return "added Sandcat block"
	}
	// Symmetric: disabling removes a previously-added block so the toggle
	// actually converges to the declared state.
	if err := gitignore.RemoveBlock(projectPath); err != nil {
		log.Warn("could not update .gitignore: %v", err)
		return "failed"
	}
	if had {
		return "removed Sandcat block (disabled)"
	}
	return "skipped (disabled)"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
