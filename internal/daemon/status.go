package daemon

import (
	"fmt"
	"net/http"

	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/internal/state"
)

type componentStatus struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	State string `json:"state"` // "running", "stopped", "unknown"
}

// handleStatus lists an app's components and whether each is running.
func (d *daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	appName := r.URL.Query().Get("name")
	if appName == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing ?name="))
		return
	}

	st, err := state.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	app, ok := st.Get(appName)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no app %q", appName))
		return
	}

	var out []componentStatus
	for _, comp := range app.Components {
		cs := componentStatus{Name: comp.Name, Image: comp.Image, State: "unknown"}
		// Pass the ID — the engine loads containers by ID, and comp.ID is
		// what was recorded when the container was created.
		status, err := d.engine.Status(r.Context(), engine.Handle{ID: comp.ID, Name: comp.Name})
		if err == nil {
			cs.State = status.State.String()
		} else {
			cs.State = "stopped"
		}
		out = append(out, cs)
	}

	writeJSON(w, http.StatusOK, out)
}
