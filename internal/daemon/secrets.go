package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/theNutsua/FastShip/internal/secrets"
)

type secretSetRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type secretGetRequest struct {
	Name string `json:"name"`
}

// handleSecretSet stores a user secret in the encrypted store.
func (d *daemon) handleSecretSet(w http.ResponseWriter, r *http.Request) {
	var req secretSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	store, err := secrets.Open()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := store.Set(req.Name, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": req.Name, "status": "set"})
}

// handleSecretGet reveals a secret's value. A deliberate action — the user
// asked for it explicitly.
func (d *daemon) handleSecretGet(w http.ResponseWriter, r *http.Request) {
	var req secretGetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	store, err := secrets.Open()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	value, ok := store.Get(req.Name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("no secret named %q", req.Name))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": req.Name, "value": value})
}

// handleSecretList returns secret NAMES only, never values.
func (d *daemon) handleSecretList(w http.ResponseWriter, r *http.Request) {
	store, err := secrets.Open()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"names": store.List()})
}
