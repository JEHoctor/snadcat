package cli

import (
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Sandcat version",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			// Root's PersistentPreRun skips the banner for this command, so
			// printing it here is what produces the single line of output.
			printBanner()
			return nil
		},
	}
}
