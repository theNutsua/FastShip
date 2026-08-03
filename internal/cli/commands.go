package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/pkg/config"
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

// runCmd tells the daemon to run an app.
//
// All the real work — build, plan, start, DNS, state — happens in the
// daemon now. The CLI just sends the request and prints the result.
var runCmd = &cobra.Command{
	Use:   "run [app]",
	Short: "Run an app locally",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		if err := clientRun(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	},
}

// clientRun sends a run request to the daemon from the current directory.
func clientRun() error {
	// The daemon is a separate process and does not share our working
	// directory, so we tell it where we are — that is where the
	// fastship.yaml and source live.
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	req := map[string]string{"dir": dir}
	var resp struct {
		App        string   `json:"app"`
		Components []string `json:"components"`
	}

	fmt.Println("→ sending to daemon...")
	if err := post("/run", req, &resp); err != nil {
		return err
	}

	fmt.Printf("\n%s is running (%d component(s))\n", resp.App, len(resp.Components))
	for _, comp := range resp.Components {
		fmt.Printf("  - %s\n", comp)
	}
	fmt.Println("stop with: fastship stop " + resp.App)
	return nil
}

// cleanup stops a set of already-started components. Used when a later
// component fails, and we need to roll back the ones that succeeded.
func cleanup(ctx context.Context, eng engine.Engine, handles []engine.Handle) {
	for _, h := range handles {
		err := eng.Stop(ctx, h, 5*1e9)
		if err != nil {
			return
		} // 5 seconds in nanoseconds
	}
}

// printConfig shows the resolved config — what the engineer wrote plus
// everything FastShip filled in. Making defaults visible matters: if
// detection or defaulting gets something wrong, they see it immediately
// instead of debugging a mystery later.
func printConfig(cfg *config.Config) {
	fmt.Printf("app:       %s\n", cfg.Name)
	fmt.Printf("port:      %d\n", cfg.Port)
	fmt.Printf("scale:     %d-%d (drain %s)\n",
		*cfg.Scale.Min, cfg.Scale.Max, cfg.Scale.DrainTimeout)
	fmt.Printf("resources: %.1f cpu, %s\n",
		cfg.Resources.CPU, cfg.Resources.Memory)

	// Empty runtime is expected right now — pkg/detect fills it in next.
	if cfg.Runtime == "" {
		fmt.Printf("runtime:   (not detected yet)\n")
	} else {
		fmt.Printf("runtime:   %s\n", cfg.Runtime)
	}

	for _, svc := range cfg.Services {
		kind := "managed"
		if !svc.Managed() {
			kind = "external → " + svc.URL
		}
		fmt.Printf("service:   %s (%s)\n", svc.Name, kind)
	}

	// Names only. A secret value must never reach terminal output.
	for _, sec := range cfg.Secrets {
		fmt.Printf("secret:    %s\n", sec.Name)
	}
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

var stopCmd = &cobra.Command{
	Use:   "stop [app]",
	Short: "Stop a running app",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		if err := clientStop(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	},
}

func clientStop(app string) error {
	req := map[string]string{"app": app}
	var resp struct {
		App    string `json:"app"`
		Status string `json:"status"`
	}

	if err := post("/stop", req, &resp); err != nil {
		return err
	}

	fmt.Printf("%s %s\n", resp.App, resp.Status)
	return nil
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
	Short: "Show logs for a running app",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		if err := clientLogs(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
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

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage secrets",
}

var secretSetCmd = &cobra.Command{
	Use:   "set [name] [value]",
	Short: "Store a secret",
	Args:  cobra.ExactArgs(2),
	Run: func(c *cobra.Command, args []string) {
		req := map[string]string{"name": args[0], "value": args[1]}
		if err := post("/secret/set", req, nil); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("secret %q set\n", args[0])
	},
}

var secretGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Reveal a secret's value",
	Args:  cobra.ExactArgs(1),
	Run: func(c *cobra.Command, args []string) {
		req := map[string]string{"name": args[0]}
		var resp struct {
			Value string `json:"value"`
		}
		if err := post("/secret/get", req, &resp); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Println(resp.Value)
	},
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secret names",
	Args:  cobra.NoArgs,
	Run: func(c *cobra.Command, args []string) {
		var resp struct {
			Names []string `json:"names"`
		}
		if err := post("/secret/list", map[string]string{}, &resp); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if len(resp.Names) == 0 {
			fmt.Println("no secrets set")
			return
		}
		for _, name := range resp.Names {
			fmt.Println(name)
		}
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

// statusCmd asks the daemon what's running.
//
// Stubbed for now — the daemon needs a /status endpoint before this can
// show real data. Wired as a placeholder so the command exists.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show all running apps",
	Args:  cobra.NoArgs,
	Run: func(c *cobra.Command, args []string) {
		fmt.Println("status — not yet implemented in the daemon")
	},
}

// scaleCmd sets an app's instance count.
//
// Stubbed for now — scaling isn't built yet.
var scaleCmd = &cobra.Command{
	Use:   "scale [app] [count]",
	Short: "Scale an app to N instances",
	Args:  cobra.ExactArgs(2),
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("scale: %s to %s — not yet implemented\n", args[0], args[1])
	},
}

func clientLogs(name string) error {
	if err := ensureDaemon(); err != nil {
		return err
	}
	client := newClient()
	resp, err := client.Get("http://localhost/logs?name=" + name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("%s", e.Error)
	}

	// Stream the logs to our stdout.
	io.Copy(os.Stdout, resp.Body)
	return nil
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
	secretCmd.AddCommand(secretSetCmd, secretGetCmd, secretListCmd)
	targetCmd.AddCommand(targetAddCmd, targetListCmd)
}
