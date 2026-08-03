package daemon

import (
	"fmt"
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
	follow := r.URL.Query().Get("follow") == "true"

	// r.Context() is cancelled when the client disconnects — passing it to
	// the engine lets a followed stream stop when the CLI is Ctrl+C'd.
	rc, err := d.engine.Logs(r.Context(), engine.Handle{Name: name}, follow)
	if err != nil {
		writeError(w, http.StatusNotFound,
			fmt.Errorf("no logs for %q (is it running?)", name))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	// Flush as we go so lines reach the client immediately, not buffered
	// until the handler returns (which, when following, is never).
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
