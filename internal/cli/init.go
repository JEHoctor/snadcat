package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/compose"
	"github.com/jehoctor/snadcat/internal/config"
	"github.com/jehoctor/snadcat/internal/devcontainer"
	"github.com/jehoctor/snadcat/internal/initialize"
	"github.com/jehoctor/snadcat/internal/project"
)

func newInitCmd() *cobra.Command {
	var opts initialize.Options

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Sandcat sandbox for a project",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			// The bash distinguishes "flag absent" (prompt) from "flag
			// empty" (e.g. --stacks "" for no stacks); Changed() carries that.
			opts.StacksProvided = c.Flags().Changed("stacks")
			opts.FeaturesProvided = c.Flags().Changed("features")
			opts.SecretProviderProvided = c.Flags().Changed("secret-provider") ||
				c.Flags().Changed("sp")
			return initialize.Run(opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.Name, "name", "", "Project name for Docker Compose")
	f.StringVar(&opts.Path, "path", "", "Project directory path (default \".\")")
	f.StringVar(&opts.Agent, "agent", "", "Agent: claude, cursor, codex")
	f.StringVar(&opts.IDE, "ide", "", "IDE: vscode, jetbrains, none")
	f.StringVar(&opts.Stacks, "stacks", "", "Comma-separated development stacks")
	f.StringVar(&opts.Proxy, "proxy", "", "Proxy UI mode: web, tui")
	f.StringVar(&opts.SecretProvider, "secret-provider", "", "Secret backend: none, 1password, protonpass")
	f.StringVar(&opts.SecretProvider, "sp", "", "Alias for --secret-provider")
	f.StringVar(&opts.Features, "features", "", "Comma-separated features: tui, no-shared-cache, no-gitignore, no-rtk, strict-network")
	f.BoolVar(&opts.OnePasswordAlias, "1password", false, "Deprecated: same as --secret-provider 1password")

	cmd.AddCommand(newInitSettingsCmd(), newInitDevcontainerCmd())
	return cmd
}

// `sandcat init settings <path>` writes only the project settings file.
func newInitSettingsCmd() *cobra.Command {
	var strict bool
	var stacksArg string
	cmd := &cobra.Command{
		Use:   "settings <path>",
		Short: "Write only the project settings file",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return config.WriteProjectSettings(args[0], config.ProjectSettingsOptions{
				StrictNetwork: strict,
				Stacks:        strings.Fields(stacksArg),
			})
		},
	}
	cmd.Flags().BoolVar(&strict, "strict-network", false, "Stack network presets instead of the allow-all-GET wildcard")
	cmd.Flags().StringVar(&stacksArg, "stacks", "", "Space-separated resolved stack names whose presets seed the strict policy")
	return cmd
}

// `sandcat init devcontainer` writes only the .devcontainer directory, with
// every choice supplied as a flag (there are no prompts at this level).
func newInitDevcontainerCmd() *cobra.Command {
	var (
		settingsFile, projectPath, agentName, ide, name, stacksArg, proxy, provider string
		onePassword                                                                 bool
	)
	cmd := &cobra.Command{
		Use:   "devcontainer",
		Short: "Write only the .devcontainer directory",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			for _, req := range []struct{ flag, value string }{
				{"--settings-file", settingsFile}, {"--project-path", projectPath}, {"--agent", agentName},
			} {
				if req.value == "" {
					return fmt.Errorf("Missing required parameter: %s", req.flag)
				}
			}
			agent, ok := agents.Get(agentName)
			if !ok {
				return fmt.Errorf("Invalid agent: %s", agentName)
			}
			if onePassword {
				provider = "1password"
			}
			if name == "" {
				name = project.DeriveName(projectPath)
			}
			abs, err := filepath.Abs(projectPath)
			if err != nil {
				return err
			}
			mounts := compose.DefaultOptions()
			mounts.Agent = agent
			mounts.ApplyEnvOverrides(os.LookupEnv)
			return devcontainer.Generate(devcontainer.Options{
				ProjectPath:    abs,
				ProjectName:    name,
				SettingsFile:   settingsFile,
				Agent:          agent,
				IDE:            ide,
				Stacks:         strings.Fields(stacksArg),
				ProxyTUI:       proxy == "tui",
				SecretProvider: compose.SecretProvider(provider),
				RTKEnabled:     os.Getenv("SANDCAT_RTK") != "false",
				Mounts:         mounts,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&settingsFile, "settings-file", "", "Settings file path, relative to the project directory")
	f.StringVar(&projectPath, "project-path", "", "Project directory")
	f.StringVar(&agentName, "agent", "", "Agent: claude, cursor, codex")
	f.StringVar(&ide, "ide", "none", "IDE: vscode, jetbrains, none")
	f.StringVar(&name, "name", "", "Project name")
	f.StringVar(&stacksArg, "stacks", "", "Space-separated resolved stack names")
	f.StringVar(&proxy, "proxy", "web", "Proxy UI mode: web, tui")
	f.StringVar(&provider, "secret-provider", "none", "Secret backend: none, 1password, protonpass")
	f.BoolVar(&onePassword, "1password", false, "Same as --secret-provider 1password")
	return cmd
}
