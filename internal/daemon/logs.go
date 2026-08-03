package daemon

import (
	"fmt"
	"io"
	"net/http"

	"github.com/theNutsua/FastShip/internal/engine"
)

// handleLogs streams a component's log file back to the CLI.
//
// The logs are captured by the engine to files under
// /var/lib/fastship/logs. This reads the requested component's file and
// writes it to the response. For now it returns the whole file; live
// following can stream new lines as they arrive.
func (d *daemon) handleLogs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing ?name="))
		return
	}

	// Ask the engine for the component's logs. The engine owns container
	// I/O, so logs come through it — not by the daemon reading files behind
	// the engine's back.
	rc, err := d.engine.Logs(r.Context(), engine.Handle{Name: name})
	if err != nil {
		writeError(w, http.StatusNotFound,
			fmt.Errorf("no logs for %q (is it running?)", name))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}
