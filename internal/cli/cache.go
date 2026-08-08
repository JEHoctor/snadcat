package cli

import (
	"github.com/spf13/cobra"
)

func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and remove the shared dependency-cache volumes",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List shared-cache volumes",
			Args:  cobra.NoArgs,
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		},
		&cobra.Command{
			Use:   "size",
			Short: "Show the on-disk size of each shared-cache volume",
			Args:  cobra.NoArgs,
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		},
		newCacheRmCmd(),
	)

	// A bare `sandcat cache` maps to libexec/cache/cache, which lists.
	cmd.RunE = func(c *cobra.Command, _ []string) error { return errNotImplemented }
	cmd.Args = cobra.NoArgs
	return cmd
}

func newCacheRmCmd() *cobra.Command {
	var all, force, yes bool
	cmd := &cobra.Command{
		Use:   "rm <volume>|--all",
		Short: "Remove one or all shared-cache volumes",
		Args:  cobra.MaximumNArgs(1),
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
	f := cmd.Flags()
	f.BoolVar(&all, "all", false, "Remove every sandcat-shared-cache volume")
	f.BoolVar(&force, "force", false, "Skip the \"container still using this volume\" check")
	f.BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}
