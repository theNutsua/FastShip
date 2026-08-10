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

	// The app is the last spec; services are everything before it. We start
	// services first, then run release commands, then start the app — so
	// migrations can reach a running database before the app serves.
	if len(plan.Specs) == 0 {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("nothing to run"))
		return
	}
	appSpec := plan.Specs[len(plan.Specs)-1]
	serviceSpecs := plan.Specs[:len(plan.Specs)-1]

	var started []engine.Handle
	var names []string

	//  Start managed services first.
	for _, spec := range serviceSpecs {
		h, err := d.engine.Start(ctx, spec)
		if err != nil {
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

	//  Run release commands in one-shot containers from the app's image,
	// after services are up but before the app starts. Migrations, asset
	// compilation, and other framework setup happen here. Any failure aborts
	// the run and rolls back the services.
	for _, releaseCmd := range cfg.Release {
		fmt.Printf("→ release: %s\n", releaseCmd)
		// Run through a shell so quotes, pipes, and && work naturally —
		// release commands are shell commands as the user would type them.
		code, err := d.engine.RunOnce(ctx, appSpec, []string{"sh", "-c", releaseCmd})
		if err != nil {
			for _, sh := range started {
				d.engine.Stop(ctx, sh, 0)
			}
			writeError(w, http.StatusInternalServerError,
				fmt.Errorf("release command %q failed: %w", releaseCmd, err))
			return
		}
		if code != 0 {
			for _, sh := range started {
				d.engine.Stop(ctx, sh, 0)
			}
			writeError(w, http.StatusInternalServerError,
				fmt.Errorf("release command %q exited with code %d — see: fastship logs %s-release",
					releaseCmd, code, cfg.Name))
			return
		}
	}

	//  Start the app itself.
	appHandle, err := d.engine.Start(ctx, appSpec)
	if err != nil {
		for _, sh := range started {
			d.engine.Stop(ctx, sh, 0)
		}
		writeError(w, http.StatusInternalServerError,
			fmt.Errorf("starting %s: %w", appSpec.Name, err))
		return
	}
	started = append(started, appHandle)
	names = append(names, appSpec.Name)

	// 6. Record state. Components are the services plus the app, in the
	// order they were started.
	st, err := state.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	allSpecs := append(serviceSpecs, appSpec)
	app := &state.App{Name: cfg.Name}
	for i, spec := range allSpecs {
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
