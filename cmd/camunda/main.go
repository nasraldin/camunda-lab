package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-dev"

func main() {
	if err := execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func execute() error {
	// wired in Task 2
	fmt.Println("camunda", version)
	return nil
}
