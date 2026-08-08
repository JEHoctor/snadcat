package cli

import (
	"github.com/spf13/cobra"
)

// InitOptions carries every `sandcat init` choice. Each field has both a flag
// and an interactive prompt; supplying the flag suppresses the prompt, which is
// what makes the differential harness able to run init non-interactively.
type InitOptions struct {
	Name           string
	Path           string
	Agent          string
	IDE            string
	Stacks         string
	Proxy          string
	SecretProvider string
	Features       string

	// The bash implementation distinguishes "flag absent" from "flag empty" for
	// these — `--stacks ""` means "no stacks, don't ask", while omitting it
	// means "prompt me". cobra's Changed() gives the same signal.
	StacksProvided         bool
	FeaturesProvided       bool
	SecretProviderProvided bool

	// OnePasswordAlias backs the deprecated --1password flag.
	OnePasswordAlias bool
}

func newInitCmd() *cobra.Command {
	var opts InitOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Sandcat sandbox for a project",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			opts.StacksProvided = c.Flags().Changed("stacks")
			opts.FeaturesProvided = c.Flags().Changed("features")
			opts.SecretProviderProvided = c.Flags().Changed("secret-provider") ||
				c.Flags().Changed("sp")
			return errNotImplemented
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
	f.StringVar(&opts.Features, "features", "", "Comma-separated features: tui, no-shared-cache, no-gitignore, no-rtk")
	f.BoolVar(&opts.OnePasswordAlias, "1password", false, "Deprecated: same as --secret-provider 1password")

	cmd.AddCommand(
		&cobra.Command{
			Use:   "settings",
			Short: "Write only the project settings file",
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		},
		&cobra.Command{
			Use:   "devcontainer",
			Short: "Write only the .devcontainer directory",
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		},
	)
	return cmd
}
