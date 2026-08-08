package main

import (
	"errors"
	"os"
	"os/exec"

	"github.com/jehoctor/snadcat/internal/cli"
	"github.com/jehoctor/snadcat/internal/log"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		log.Error("%s", err)
		// Propagate the exit code of a wrapped docker/git invocation so
		// `sandcat compose ...` is transparent to scripts and CI.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		os.Exit(1)
	}
}
