package cli

import (
	"github.com/spf13/cobra"

	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/version"
)

// NewRootCmd builds the full command tree.
//
// The bash dispatcher (cli/bin/sandcat) resolves `sandcat <module> [command]`
// against libexec/<module>/<command>, with a `_` catch-all and a bare
// <module> defaulting to the same-named command. The cobra tree below mirrors
// that surface one-for-one so documented invocations keep working; see
// plans/2026-08-07-go-port.md §1.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "sandcat",
		Short: "Docker & dev container setup for securely running AI agents",
		// Errors are reported by main with the right exit code; cobra's own
		// usage dump on every failure is noise.
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			// bin/sandcat prints the version banner before dispatching to any
			// module except `version` (which prints it as its whole job).
			if rootCommandName(cmd) == "version" {
				return
			}
			printBanner()
		},
	}

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newRunCmd(),
		newComposeCmd(),
		newAttachCmd(),
		newDestroyCmd(),
		newProxyCmd(),
		newRestartCmd(),
		newRestartProxyCmd(),
		newCacheCmd(),
		newEditCmd(),
	)
	return root
}

// rootCommandName returns the name of the top-level command cmd sits under,
// i.e. the bash "module".
func rootCommandName(cmd *cobra.Command) string {
	for cmd.Parent() != nil && cmd.Parent().Parent() != nil {
		cmd = cmd.Parent()
	}
	return cmd.Name()
}

func printBanner() {
	if v := version.String(); v != "" {
		log.Info("%s %s", version.Name, v)
	} else {
		log.Warn("cannot determine version")
	}
}
