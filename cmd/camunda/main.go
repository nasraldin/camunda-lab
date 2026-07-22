package main

import (
	"context"
	"os"

	"github.com/nasraldin/camunda-lab/internal/cli"
)

var version = "0.0.0-dev"

func main() {
	cli.SetVersion(version)
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
