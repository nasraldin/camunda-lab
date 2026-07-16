package main

import (
	"fmt"
	"os"

	"github.com/nasraldin/camunda-lab/internal/cli"
)

var version = "0.0.0-dev"

func main() {
	cli.SetVersion(version)
	if err := cli.NewRoot().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
