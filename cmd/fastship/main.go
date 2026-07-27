// This file stays deliberately tiny. All command logic lives in
// internal/cli so that the binary itself is just a launcher.
package main

import "github.com/theNutsua/FastShip/internal/cli"

func main() {
	// Execute parses os.Args, finds the matching command, and runs it.
	// It handles its own exit codes, so nothing to do here
	cli.Execute()
}
