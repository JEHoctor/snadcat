package main

import (
	"errors"
	"os"

	"github.com/jehoctor/snadcat/internal/cli"
	"github.com/jehoctor/snadcat/internal/dockercli"
	"github.com/jehoctor/snadcat/internal/log"
)

func main() {
	err := cli.NewRootCmd().Execute()
	if err == nil {
		return
	}
	// A wrapped docker invocation already printed its own diagnostics;
	// propagate its status so `sandcat compose ...` is transparent to
	// scripts and CI, without a second message on top.
	var ee dockercli.ExitError
	if errors.As(err, &ee) {
		os.Exit(ee.Code)
	}
	log.Error("%s", err)
	os.Exit(1)
}
