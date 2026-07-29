package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/theNutsua/FastShip/internal/build"
	"github.com/theNutsua/FastShip/internal/engine"
	"github.com/theNutsua/FastShip/internal/planner"
	"github.com/theNutsua/FastShip/internal/state"
	"github.com/theNutsua/FastShip/pkg/config"
	"github.com/theNutsua/FastShip/pkg/detect"
)

// runRequest is what the CLI sends to /run.
type runRequest struct {
	// Dir is the working directory the CLI was run from — the daemon needs
	// it to find the fastship.yaml and the source to build. The daemon runs
	// as a separate process, so it does not share the CLI's directory; the
	// CLI must tell it where to look.
	Dir string `json:"dir"`
}

// runResponse is what the daemon sends back.
type runResponse struct {
	App        string   `json:"app"`
	Components []string `json:"components"`
}

// handleRun does what runApp used to do, but inside the daemon: load
// config, detect, plan, build, start containers, record state. Because
// the engine (and its DNS server) live in the daemon, everything started
// here persists after the CLI request returns.
func (d *daemon) handleRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("bad request: %w", err))
		return
	}

	ctx := context.Background()

	// 1. Load config from the directory the CLI told us about.
	cfg, err := config.Load(req.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// 2. Detect.
	if _, err := detect.Apply(req.Dir, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// 3. Plan.
	plan, err := planner.Build(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 4. Build the app image.
	builder := build.New()
	buildResult, err := builder.Build(ctx, req.Dir, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("building: %w", err))
		return
	}
	for i := range plan.Specs {
		if plan.Specs[i].Image == "" {
			plan.Specs[i].Image = buildResult.ImageRef
		}
	}

	// 5. Start each spec through the daemon's long-lived engine.
	var started []engine.Handle
	var names []string
	for _, spec := range plan.Specs {
		h, err := d.engine.Start(ctx, spec)
		if err != nil {
			// Roll back what we started.
			for _, sh := range started {
				d.engine.Stop(ctx, sh, 0)
			}
			writeError(w, http.StatusInternalServerError,
				fmt.Errorf("starting %s: %w", spec.Name, err))
			return
		}
		started = append(started, h)
		names = append(names, spec.Name)
	}

	// 6. Record state.
	st, err := state.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	app := &state.App{Name: cfg.Name}
	for i, spec := range plan.Specs {
		app.Components = append(app.Components, state.Component{
			Name: spec.Name, ID: started[i].ID, Image: spec.Image,
		})
	}
	if err := st.Put(app); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, runResponse{App: cfg.Name, Components: names})
}

// writeError sends an error as JSON so the CLI can display it.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
