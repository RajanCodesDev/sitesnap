// Command sitesnap is a fast concurrent website snapshot tool for verifying
// deployments. See the internal/cli package for behavior.
package main

import (
	"fmt"
	"os"

	"sitesnap/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "sitesnap: %v\n", err)
		os.Exit(1)
	}
}
