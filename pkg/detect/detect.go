// Package detect infers what an engineer left out of ship.yaml.
// The product promise is that if FastShip can figure something out, the
// engineer should not have to write it. This package is where that
// happens: it scans a repository and fills in Runtime, Start, and Port.
// Two rules govern everything here:
//  1. Explicit always wins. A value already set in ship.yaml is never
//     overwritten, no matter what the scan finds.
//  2. Guessing silently is worse than failing loudly. When detection is
//     ambiguous, return an error naming what to add to ship.yaml rather
//     than picking one and hoping.
package detect

import (
	"fmt"

	"github.com/theNutsua/FastShip/pkg/config"
)

// Result is what a scan found. It is returned separately from the Config
// so callers can show the engineer what was inferred before applying it.
type Result struct {
	Runtime string // e.g. "node@18"
	Start   string // e.g. "npm start"
	Port    int    // e.g. 3000
}

// Apply fills in a Config's empty fields from a repository scan.
//
// repoPath is the directory to scan — usually the app's Path from
// ship.yaml, or "." for a single-app project.
//
// Fields already set in the Config are left untouched.
func Apply(repoPath string, cfg *config.Config) (*Result, error) {
	res := &Result{
		Runtime: cfg.Runtime,
		Start:   cfg.Start,
		Port:    cfg.Port,
	}

	// Runtime first — the other two detections depend on knowing it.
	if res.Runtime == "" {
		rt, err := DetectRuntime(repoPath)
		if err != nil {
			return nil, err
		}
		res.Runtime = rt
	}

	if res.Start == "" {
		start, err := DetectStart(repoPath, res.Runtime)
		if err != nil {
			return nil, err
		}
		res.Start = start
	}

	if res.Port == 0 {
		// Port detection never errors. A port of 0 is legitimate — it means
		// an internal-only service that gets no external route.
		res.Port = DetectPort(repoPath, res.Runtime)
	}

	cfg.Runtime = res.Runtime
	cfg.Start = res.Start
	cfg.Port = res.Port

	return res, nil
}

// String renders a Result for the one-line summary FastShip prints on
// startup. Showing detections is not optional — if a guess is wrong the
// engineer must see it immediately, not discover it later.
func (r *Result) String() string {
	s := fmt.Sprintf("detected %s", r.Runtime)
	if r.Start != "" {
		s += fmt.Sprintf(", start: %s", r.Start)
	}
	if r.Port > 0 {
		s += fmt.Sprintf(", port %d", r.Port)
	}
	return s
}
