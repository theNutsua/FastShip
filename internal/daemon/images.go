package daemon

import (
	"fmt"
	"net/http"

	"github.com/theNutsua/FastShip/internal/state"
)

type imageInfo struct {
	Component string `json:"component"`
	Image     string `json:"image"`
	SizeBytes int64  `json:"size_bytes"`
}

// handleImages returns the image size for each of an app's components,
// so the debug view can show how small FastShip's images are.
func (d *daemon) handleImages(w http.ResponseWriter, r *http.Request) {
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

	var out []imageInfo
	for _, comp := range app.Components {
		info := imageInfo{Component: comp.Name, Image: comp.Image}
		// Ask the engine for the image's size.
		size, err := d.engine.ImageSize(r.Context(), comp.Image)
		if err == nil {
			info.SizeBytes = size
		}
		out = append(out, info)
	}

	writeJSON(w, http.StatusOK, out)
}

// formatBytes turns a raw byte count into a human-readable size like
// "5.0 MB" or "116 MB". Used wherever sizes are shown to a person —
// the raw bytes stay in the API, the friendly string is for display.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
