package main

import (
	"fmt"
	"os"

	"github.com/gautambaghel/hsdebug/internal/cli"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	if err := cli.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "hsdebug: "+err.Error())
		os.Exit(1)
	}
}
