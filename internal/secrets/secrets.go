package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// storePath is where the encrypted secrets live — separate from state.json
// so secrets never leak into a state dump.
func storePath() string {
	return filepath.Join(os.Getenv("HOME"), ".fastship", "secrets.enc")
}

// Store is the encrypted secret store. Values are held in memory decrypted
// while the store is open, and written back encrypted on every change.
type Store struct {
	mu     sync.RWMutex
	key    []byte
	values map[string]string
}

// Open loads the secret store, decrypting it, creating an empty one on
// first use. The key is loaded or created here.
func Open() (*Store, error) {
	key, err := loadOrCreateKey()
	if err != nil {
		return nil, err
	}

	s := &Store{key: key, values: map[string]string{}}

	// Load and decrypt the existing store, if any.
	sealed, err := os.ReadFile(storePath())
	if os.IsNotExist(err) {
		return s, nil // fresh store
	}
	if err != nil {
		return nil, fmt.Errorf("reading secret store: %w", err)
	}

	plaintext, err := decrypt(key, sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypting secret store: %w", err)
	}
	if err := json.Unmarshal(plaintext, &s.values); err != nil {
		return nil, fmt.Errorf("parsing secret store: %w", err)
	}

	return s, nil
}

// Set stores a secret and persists the store encrypted.
func (s *Store) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = value
	return s.save()
}

// Get returns a secret's value and whether it exists.
func (s *Store) Get(name string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[name]
	return v, ok
}

// Delete removes a secret.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, name)
	return s.save()
}

// List returns the NAMES of all secrets — never the values. This is what
// "fastship secret list" shows: what exists, without exposing anything.
func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.values))
	for name := range s.values {
		names = append(names, name)
	}
	return names
}

// save encrypts the whole store and writes it atomically. Called under the
// lock by every mutation.
func (s *Store) save() error {
	plaintext, err := json.Marshal(s.values)
	if err != nil {
		return err
	}

	sealed, err := encrypt(s.key, plaintext)
	if err != nil {
		return err
	}

	// Atomic write: temp file then rename, so a crash mid-write never
	// leaves a half-written (undecryptable) store.
	path := storePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
