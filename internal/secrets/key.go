// Package secrets provides an encrypted store for sensitive values
// generated database passwords, user-supplied API keys so they never
// sit in plaintext on disk.
//
// The threat model is local and honest: this protects against the common
// way secrets leak a stray cat, a backup, a committed file, a log not
// against an attacker who already has root on the machine (who could read
// the key and the daemon's memory regardless). Defending that needs
// hardware key management, which is a later, enterprise concern.
package secrets

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

// keyPath is where the encryption key lives. Tight permissions (0600)
// restrict it to the owner — the daemon runs as root, so only root reads
// it.
func keyPath() string {
	return filepath.Join(os.Getenv("HOME"), ".fastship", "key")
}

// loadOrCreateKey returns the 32-byte AES-256 key, creating it on first
// use. The key is generated once and reused; if it changed, every stored
// secret would become undecryptable.
func loadOrCreateKey() ([]byte, error) {
	path := keyPath()

	// Try to load an existing key.
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("key file %s is corrupt (wrong size)", path)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading key: %w", err)
	}

	// No key yet — generate a fresh 32-byte (AES-256) key.
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	// Write it with owner-only permissions.
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating key dir: %w", err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("writing key: %w", err)
	}

	return key, nil
}
