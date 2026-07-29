package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/internal/state"
)

type stopRequest struct {
	App string `json:"app"`
}

func (d *daemon) handleStop(w http.ResponseWriter, r *http.Request) {
	var req stopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("bad request: %w", err))
		return
	}

	ctx := context.Background()

	st, err := state.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	app, ok := st.Get(req.App)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no running app named %q", req.App))
		return
	}

	for _, comp := range app.Components {
		h := engine.Handle{ID: comp.ID, Name: comp.Name}
		d.engine.Stop(ctx, h, 30*time.Second)
	}

	if err := st.Remove(req.App); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"app": req.App, "status": "stopped"})
}
