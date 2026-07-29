package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/internal/engine/containerd"
)

const socketPath = "/run/fastship/shipd.sock"

// daemon holds the long-lived state: the engine (which carries the DNS
// server and network), alive for the whole process lifetime. This is the
// thing that persists — the entire reason shipd exists.
type daemon struct {
	engine engine.Engine
}

// Run starts the daemon and serves until interrupted.
func Run() error {
	// Create the engine ONCE, here. It stays alive as long as the daemon
	// does — so the DNS server it started keeps running across every CLI
	// command, which is what the whole daemon split is for.
	cdEngine, err := containerd.New()
	if err != nil {
		return fmt.Errorf("starting engine: %w", err)
	}
	defer cdEngine.Close()

	d := &daemon{engine: cdEngine}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return fmt.Errorf("creating socket dir: %w", err)
	}

	// Make the directory group-accessible so a non-root CLI can reach the
	// socket inside it.
	if grp, err := user.LookupGroup("fastship"); err == nil {
		if gid, err := strconv.Atoi(grp.Gid); err == nil {
			os.Chown(filepath.Dir(socketPath), 0, gid)
			os.Chmod(filepath.Dir(socketPath), 0750)
		}
	}

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", socketPath, err)
	}
	defer listener.Close()

	// Make the socket accessible to the fastship group so the CLI does not
	// need sudo.
	if err := setSocketPermissions(socketPath); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/run", d.handleRun) // methods on d, so they can use the engine
	mux.HandleFunc("/stop", d.handleStop)

	fmt.Printf("shipd listening on %s\n", socketPath)
	return http.Serve(listener, mux)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// setSocketPermissions makes the daemon socket accessible to the fastship
// group, so the CLI can talk to the daemon WITHOUT sudo. The daemon still
// runs as root (it needs root for containerd and CNI), but this socket is
// the one door a normal user is allowed through — gated by group
// membership, exactly like Docker's socket.
func setSocketPermissions(path string) error {
	// Look up the fastship group's ID.
	grp, err := user.LookupGroup("fastship")
	if err != nil {
		// Group does not exist — skip silently. The daemon still works via
		// sudo; this is only the no-sudo convenience.
		return nil
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return err
	}

	// Set the socket's group to fastship.
	if err := os.Chown(path, 0, gid); err != nil {
		return fmt.Errorf("setting socket group: %w", err)
	}

	// Make it group read/write (0660): owner (root) and group (fastship)
	// can use it; everyone else cannot.
	if err := os.Chmod(path, 0660); err != nil {
		return fmt.Errorf("setting socket permissions: %w", err)
	}

	return nil
}
