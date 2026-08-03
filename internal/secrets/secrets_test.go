package secrets

import (
	"bytes"
	"os"
	"testing"
)

// TestEncryptDecryptRoundTrip proves the core guarantee: what goes in
// comes back out, and only with the right key.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("sk_live_super_secret_api_key")

	sealed, err := encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// The sealed output must NOT contain the plaintext — that's the whole point.
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("plaintext is visible in the ciphertext!")
	}

	got, err := decrypt(key, sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip failed: got %q, want %q", got, plaintext)
	}
}

// TestWrongKeyFails proves a wrong key cannot decrypt — no silent garbage.
func TestWrongKeyFails(t *testing.T) {
	key1 := bytes.Repeat([]byte{1}, 32)
	key2 := bytes.Repeat([]byte{2}, 32)

	sealed, err := encrypt(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if _, err := decrypt(key2, sealed); err == nil {
		t.Fatal("decryption with the wrong key should have failed")
	}
}

// TestTamperingDetected proves altered ciphertext is rejected, not
// silently returned as corrupt data.
func TestTamperingDetected(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)

	sealed, err := encrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Flip a byte in the ciphertext.
	sealed[len(sealed)-1] ^= 0xff

	if _, err := decrypt(key, sealed); err == nil {
		t.Fatal("tampered ciphertext should have failed to decrypt")
	}
}

// TestStoreRoundTrip proves a secret set in one store is readable after
// reopening — it really persisted, encrypted, and came back.
func TestStoreRoundTrip(t *testing.T) {
	// Use a temp HOME so the test does not touch the real store.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	s1, err := Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s1.Set("STRIPE_KEY", "sk_live_abc123"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Reopen — this reads and decrypts from disk.
	s2, err := Open()
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := s2.Get("STRIPE_KEY")
	if !ok {
		t.Fatal("secret not found after reopen")
	}
	if got != "sk_live_abc123" {
		t.Fatalf("got %q, want sk_live_abc123", got)
	}

	// List shows the name.
	names := s2.List()
	if len(names) != 1 || names[0] != "STRIPE_KEY" {
		t.Fatalf("list = %v, want [STRIPE_KEY]", names)
	}
}

// TestStoreFileIsEncrypted proves the value is NOT readable on disk.
func TestStoreFileIsEncrypted(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	s, err := Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s.Set("API_KEY", "super_secret_value")

	// Read the raw store file — the secret must NOT appear in plaintext.
	raw, err := os.ReadFile(storePath())
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if bytes.Contains(raw, []byte("super_secret_value")) {
		t.Fatal("secret value is stored in PLAINTEXT on disk!")
	}
}
