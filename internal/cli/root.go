// Package cli contains every ship command and the wiring that
// connects them together.
//
// Structure:
//
//	root.go     the root command and registration of all children
//	commands.go the individual command definitions
//
// The CLI is intentionally thin. Commands parse input and delegate
// to the daemon (shipd). No container logic belongs in this package.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd is the base command, it is what runs when someone types
// "ship" with no arguments. It has no Run function of its own, so
// calling it bare prints the help text listing all subcommands.
var rootCmd = &cobra.Command{
	Use:   "fastship",
	Short: "One tool. Local to production.",
}

// Execute is the single entry point called from main.go.
// It walks the command tree, matches the user's input to a command,
// validates arguments, and runs it. Any error has already been
// printed by then, so we only need to set a non-zero exit code.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// init registers every top level command with the root.
// Anything not registered here is invisible to the user, even if the
// command variable exists. Adding a new ship command means adding it
// to this list.
func init() {
	rootCmd.AddCommand(
		// app lifecycle
		runCmd,
		deployCmd,
		stopCmd,
		statusCmd,
		scaleCmd,

		// observability
		debugCmd,
		logsCmd,
		shellCmd,

		// commands with their own subcommands
		secretCmd,
		targetCmd,
	)
}
