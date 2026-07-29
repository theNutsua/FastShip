// Package state persists what FastShip is running, so commands in one
// invocation can act on what a previous invocation started.
//
// The problem it solves: "fastship run" starts containers and exits. Later,
// "fastship stop" is a brand-new process with no memory of what run did. It
// needs to look up which components belong to an app. That record lives
// here, on disk.
//
// Phase 1 uses a simple JSON file. It is human-readable on purpose — an
// engineer debugging a stuck deploy can cat it and see exactly what
// FastShip thinks is running. bbolt could store this faster, but a file
// you can read with your eyes is worth more than speed at this scale.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Component is one running piece of an app — the app itself or one of its
// managed services.
type Component struct {
	Name  string `json:"name"`  // component + DNS name, e.g. "postgres"
	ID    string `json:"id"`    // the engine handle ID
	Image string `json:"image"` // what image it is running
}

// App is everything FastShip started for one application.
type App struct {
	Name       string      `json:"name"`
	Components []Component `json:"components"`
}

// Store is the on-disk record of every running app.
// Creds maps "app/service" to its credentials.
type Store struct {
	path  string
	Apps  map[string]*App        `json:"apps"`
	Creds map[string]Credentials `json:"creds"` // ← add this field
}

// Credentials for a managed service, persisted so they stay stable across
// runs — the service's data (initialized with these credentials) remains
// accessible. Stored in plain state for now; moves to the encrypted
// secrets store when that exists.
type Credentials struct {
	User string `json:"user"`
	Pass string `json:"pass"`
	DB   string `json:"db"`
}

// dir returns FastShip's state directory, creating it if needed.
// Everything FastShip persists lives under ~/.fastship.
func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".fastship")
	if err := os.MkdirAll(d, 0755); err != nil {
		return "", err
	}
	return d, nil
}

// Load reads the state file from disk. A missing file is not an error —
// it just means nothing has been run yet, so we return an empty store.
func Load() (*Store, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(d, "state.json")

	s := &Store{path: path, Apps: map[string]*App{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // nothing run yet — empty store is correct
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	// Unmarshal does not set the unexported path field, so restore it.
	s.path = path
	if s.Apps == nil {
		s.Apps = map[string]*App{}
	}
	if s.Creds == nil {
		s.Creds = map[string]Credentials{}
	}

	return s, nil
}

// save writes the store back to disk atomically.
//
// Atomic means: write to a temp file, then rename it into place. A rename
// is atomic at the OS level, so a crash mid-write can never leave a
// half-written, corrupt state file. You either have the old file or the
// new one, never a broken hybrid.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Put records an app and everything it started, then saves.
func (s *Store) Put(app *App) error {
	s.Apps[app.Name] = app
	return s.save()
}

// Get returns a recorded app, and whether it was found.
func (s *Store) Get(name string) (*App, bool) {
	app, ok := s.Apps[name]
	return app, ok
}

// Remove deletes an app record and saves. Called after stop succeeds.
func (s *Store) Remove(name string) error {
	delete(s.Apps, name)
	return s.save()
}

// List returns every recorded app. Used by "fastship status".
func (s *Store) List() []*App {
	apps := make([]*App, 0, len(s.Apps))
	for _, app := range s.Apps {
		apps = append(apps, app)
	}
	return apps
}

// GetCreds returns stored credentials for an app's service, and whether
// they existed.
func (s *Store) GetCreds(app, service string) (Credentials, bool) {
	c, ok := s.Creds[app+"/"+service]
	return c, ok
}

// PutCreds stores credentials and saves.
func (s *Store) PutCreds(app, service string, c Credentials) error {
	if s.Creds == nil {
		s.Creds = map[string]Credentials{}
	}
	s.Creds[app+"/"+service] = c
	return s.save()
}
