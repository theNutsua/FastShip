package planner

import (
	"crypto/rand"
	"encoding/hex"
)

// credentials are the generated secrets for a managed service. They are
// created once and persisted (in state for now; the secrets store later),
// so a service keeps the same credentials across restarts and its data
// stays accessible.
type credentials struct {
	User string
	Pass string
	DB   string
}

// generateCredentials creates fresh credentials for a service. The
// password is random — never a hardcoded default like "changeme". The
// user and db names are derived from the app so they are stable and
// meaningful.
func generateCredentials(appName string) credentials {
	return credentials{
		User: appName,
		Pass: randomSecret(24),
		DB:   appName,
	}
}

// randomSecret returns a cryptographically random hex string of n bytes.
func randomSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is catastrophic and unrecoverable; a service
		// with a predictable password is worse than a crash.
		panic("could not generate secure random: " + err.Error())
	}
	return hex.EncodeToString(b)
}
