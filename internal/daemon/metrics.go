package daemon

import (
	"fmt"
	"net/http"

	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/internal/state"
)

// componentMetrics is one component's usage in the response.
type componentMetrics struct {
	Name             string  `json:"name"`
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryBytes      uint64  `json:"memory_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
}

// handleMetrics returns resource usage for every component of an app.
func (d *daemon) handleMetrics(w http.ResponseWriter, r *http.Request) {
	appName := r.URL.Query().Get("name")
	if appName == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing ?name="))
		return
	}

	// Look up the app's components from state.
	st, err := state.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	app, ok := st.Get(appName)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no running app %q", appName))
		return
	}

	// Gather metrics per component. A component that has stopped simply
	// reports zeros rather than failing the whole request.
	var out []componentMetrics
	for _, comp := range app.Components {
		m, err := d.engine.Metrics(r.Context(), engine.Handle{Name: comp.Name})
		cm := componentMetrics{Name: comp.Name}
		if err == nil {
			cm.CPUPercent = m.CPUPercent
			cm.MemoryBytes = m.MemoryBytes
			cm.MemoryLimitBytes = m.MemoryLimitBytes
		}
		out = append(out, cm)
	}

	writeJSON(w, http.StatusOK, out)
}
