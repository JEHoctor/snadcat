package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jehoctor/snadcat/internal/dockercli"
	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/prompt"
)

// The cache commands inspect and remove the host-scoped shared dependency
// cache volumes declared by compose.AddSharedCacheVolumes and created lazily
// by `sandcat run`. Their output is plain stdout, not the log stream: it is
// a report, and users pipe it.

func newCacheCmd() *cobra.Command {
	list := newCacheListCmd()
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and remove the shared dependency-cache volumes",
		Args:  cobra.NoArgs,
		// A bare `sandcat cache` lists, as libexec/cache/cache does.
		RunE: list.RunE,
	}
	cmd.AddCommand(list, newCacheSizeCmd(), newCacheRmCmd())
	return cmd
}

func docker() (dockercli.Docker, error) {
	if err := dockercli.Require(); err != nil {
		return dockercli.Docker{}, err
	}
	return dockercli.Docker{}, nil
}

func newCacheListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List shared-cache volumes",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			d, err := docker()
			if err != nil {
				return err
			}
			names := dockercli.CacheVolumeNames(d)
			if len(names) == 0 {
				fmt.Println("No sandcat shared-cache volumes on this host yet.")
				fmt.Println("They are created lazily on the first `sandcat run`.")
				return nil
			}

			const row = "%-32s %10s %8s  %s\n"
			fmt.Printf(row, "VOLUME", "SIZE", "FILES", "IN USE BY")
			var total int64
			for _, name := range names {
				size := dockercli.CacheVolumeSizeBytes(d, name)
				total += size
				users := strings.Join(dockercli.CacheVolumeContainers(d, name), ",")
				if users == "" {
					users = "—"
				}
				fmt.Printf(row, name, dockercli.FormatBytes(size),
					fmt.Sprint(dockercli.CacheVolumeFileCount(d, name)), users)
			}
			fmt.Printf("%-32s %10s\n", "", dockercli.FormatBytes(total)+" total")
			return nil
		},
	}
}

func newCacheSizeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "size",
		Short: "Show the total on-disk size of the shared-cache volumes",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			d, err := docker()
			if err != nil {
				return err
			}
			names := dockercli.CacheVolumeNames(d)
			if len(names) == 0 {
				fmt.Println("0 B (no shared-cache volumes on this host)")
				return nil
			}
			var total int64
			for _, name := range names {
				total += dockercli.CacheVolumeSizeBytes(d, name)
			}
			fmt.Printf("%s across %d volume(s)\n", dockercli.FormatBytes(total), len(names))
			return nil
		},
	}
}

func newCacheRmCmd() *cobra.Command {
	var all, force, yes bool
	cmd := &cobra.Command{
		Use:   "rm <volume>|--all",
		Short: "Remove one or all shared-cache volumes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			d, err := docker()
			if err != nil {
				return err
			}

			var targets []string
			switch {
			case all && len(args) > 0:
				return fmt.Errorf("Use --all OR a volume name, not both")
			case all:
				targets = dockercli.CacheVolumeNames(d)
			case len(args) == 0:
				return c.Usage()
			default:
				targets = args
			}
			if len(targets) == 0 {
				fmt.Println("No matching shared-cache volumes found.")
				return nil
			}

			// In-use check before the prompt, so nobody confirms a removal
			// that was going to fail anyway.
			if !force {
				for _, name := range targets {
					if users := dockercli.CacheVolumeContainers(d, name); len(users) > 0 {
						log.Error("Cannot remove %s — in use by: %s", name, strings.Join(users, ","))
						log.Error("Stop those containers first, or pass --force.")
						return dockercli.ExitError{Code: 1}
					}
				}
			}

			if !yes {
				fmt.Println("The following shared-cache volumes will be removed from the host:")
				for _, name := range targets {
					fmt.Printf("  - %s\n", name)
				}
				fmt.Println("This affects every sandcat sandbox that mounts them.")
				old := prompt.Out
				prompt.Out = os.Stdout // the bash prompts on stdout here
				ok, err := prompt.YesNo("Continue?")
				prompt.Out = old
				if err != nil {
					return err
				}
				if !ok {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			rmArgs := []string{"volume", "rm"}
			if force {
				rmArgs = append(rmArgs, "--force")
			}
			return childExit(d.Run(append(rmArgs, targets...)...))
		},
	}
	f := cmd.Flags()
	f.BoolVar(&all, "all", false, "Remove every sandcat-shared-cache volume")
	f.BoolVar(&force, "force", false, "Skip the \"container still using this volume\" check")
	f.BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}
