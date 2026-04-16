package main

import (
	"fmt"
	"os"

	"github.com/redhat-et/oci-skill-registry/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.NewRootCmd(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
