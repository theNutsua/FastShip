package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Every command below follows the same shape:
//
//   Use    the usage string. The first word is the command name.
//          Anything in [brackets] documents an expected argument.
//   Short  one-line description shown in "ship --help".
//   Args   validates argument count before Run is called.
//   Run    the actual work. Currently, stubbed.
// All Run bodies print a placeholder for now. They will be replaced
// with real calls to the shipd daemon over gRPC once it exists.

// App lifecycle

// runCmd starts an app on the local machine.
// Eventually this will: parse ship.yaml, detect the runtime, build
// the image, start managed services, inject secrets, and run the
// container all from this one command.
var runCmd = &cobra.Command{
	Use:   "run [app]",
	Short: "Run an app locally",
	Args:  cobra.ExactArgs(1), // exactly one arg: the app name
	Run: func(c *cobra.Command, args []string) {
		appName := args[0]
		fmt.Printf("run: %s — not implemented\n", appName)
	},
}

// deployCmd ships an app to a registered target server.
// Unlike run, deploy applies production hardening: non-root user,
// read-only filesystem, dropped capabilities, and mTLS between services.
var deployCmd = &cobra.Command{
	Use:   "deploy [app]",
	Short: "Deploy an app to a target",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		appName := args[0]

		// Flags are read inside Run, not at package level, because
		// they are only populated after cobra parses the command line.
		target, _ := c.Flags().GetString("target")

		fmt.Printf("deploy: %s to %s — not implemented\n", appName, target)
	},
}

// stopCmd gracefully shuts down a running app.
// Real implementation will drain in-flight requests before sending
// SIGTERM, so no request is dropped during shutdown.
var stopCmd = &cobra.Command{
	Use:   "stop [app]",
	Short: "Stop a running app",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("stop: %s — not implemented\n", args[0])
	},
}

// statusCmd lists every app ship knows about and its health.
// Takes no arguments — it always reports on everything.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show all running apps",
	Args:  cobra.NoArgs, // reject any arguments
	Run: func(c *cobra.Command, args []string) {
		fmt.Println("status — not implemented")
	},
}

// scaleCmd manually sets the instance count for an app.
// This is the manual override. Auto-scaling based on CPU, memory,
// and queue depth arrives in Phase 2.
var scaleCmd = &cobra.Command{
	Use:   "scale [app] [count]",
	Short: "Scale an app to N instances",
	Args:  cobra.ExactArgs(2), // app name and instance count
	Run: func(c *cobra.Command, args []string) {
		appName, count := args[0], args[1]
		fmt.Printf("scale: %s to %s — not implemented\n", appName, count)
	},
}

// Observability

// debugCmd opens the full four-panel TUI: services, logs, metrics,
// and network traffic in one view.

// This is ship's headline feature — the thing that replaces having
// four terminals open at once.

var debugCmd = &cobra.Command{
	Use:   "debug [app]",
	Short: "Open the debug TUI",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("debug: %s — not implemented\n", args[0])
	},
}

// logsCmd streams logs without opening the full TUI.
// For engineers who want a focused view they can pipe or grep.
var logsCmd = &cobra.Command{
	Use:   "logs [app]",
	Short: "Stream logs from an app",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("logs: %s — not implemented\n", args[0])
	},
}

// shellCmd opens an interactive shell inside a running container.
// Context-aware in the real implementation: selecting postgres opens
// psql already connected, redis opens redis-cli, and so on.
var shellCmd = &cobra.Command{
	Use:   "shell [app]",
	Short: "Open a shell inside a container",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("shell: %s — not implemented\n", args[0])
	},
}

// Secrets

// secretsCmd is a parent with no Run of its own. Typing
// "ship secrets" prints help listing its subcommands.

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage secrets",
}

// secretsSetCmd stores an encrypted secret.

// The value is encrypted at rest and injected into containers at
// runtime. It never touches ship.yaml, git, or any log output.
var secretsSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a secret",
	Args:  cobra.ExactArgs(2),
	Run: func(c *cobra.Command, args []string) {
		key := args[0]
		// Deliberately printing only the key, never args[1].
		// A secret value must never appear in terminal output.
		fmt.Printf("secrets set: %s — not implemented\n", key)
	},
}

// secretsRotateCmd replaces a secret's value with zero downtime.

// The old value stays valid for a short drain window while the new
// one propagates, so running containers never see a gap.
var secretsRotateCmd = &cobra.Command{
	Use:   "rotate [key]",
	Short: "Rotate a secret",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("secrets rotate: %s — not implemented\n", args[0])
	},
}

// secretsListCmd shows secret names only — never values.
var secretsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all secrets",
	Args:  cobra.NoArgs,
	Run: func(c *cobra.Command, args []string) {
		fmt.Println("secrets list — not implemented")
	},
}

// Targets
// A "target" is a server ship can deploy to. Phase 1 uses plain SSH,
// so a target is just a name, an IP, and a key.

var targetCmd = &cobra.Command{
	Use:   "target",
	Short: "Manage deployment targets",
}

// targetAddCmd registers a server for deployment.

// The real implementation SSHes in, installs containerd and shipd,
// and starts the daemon — so one command turns a bare box into a
// ship-ready server.
var targetAddCmd = &cobra.Command{
	Use:   "add [name] [ip]",
	Short: "Register a server as a deploy target",
	Args:  cobra.ExactArgs(2),
	Run: func(c *cobra.Command, args []string) {
		name, ip := args[0], args[1]
		fmt.Printf("target add: %s at %s — not implemented\n", name, ip)
	},
}

// targetListCmd shows every registered target.
var targetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered targets",
	Args:  cobra.NoArgs,
	Run: func(c *cobra.Command, args []string) {
		fmt.Println("target list — not implemented")
	},
}

// Wiring

// init attaches flags and connects subcommands to their parents.
//
// This runs automatically before main(), so everything is wired up
// by the time Execute is called.
func init() {
	// StringP takes: long name, short name, default value, help text.
	// So --target and -t both work.
	deployCmd.Flags().StringP("target", "t", "", "target server to deploy to")
	deployCmd.Flags().StringP("env", "e", "prod", "environment")

	// debug defaults to local since that is where it is used most.
	debugCmd.Flags().StringP("env", "e", "local", "environment")

	// Attach subcommands. Without this, "ship secrets set" is unknown.
	secretsCmd.AddCommand(secretsSetCmd, secretsRotateCmd, secretsListCmd)
	targetCmd.AddCommand(targetAddCmd, targetListCmd)
}
