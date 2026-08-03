package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// encrypt seals plaintext with AES-256-GCM. GCM is authenticated
// encryption: it both hides the data and detects tampering if the
// ciphertext is altered, decryption fails rather than returning garbage.
//
// The nonce (a unique number per encryption) is prepended to the output,
// so decrypt can find it. A fresh random nonce every time is essential:
// reusing one with the same key breaks GCM's security.
func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// A unique random nonce for this encryption.
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal prepends the nonce (via the first arg) and appends the auth tag.
	// Result layout: [nonce][ciphertext+tag].
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt reverses encrypt. It splits the nonce back off the front, then
// verifies and decrypts. If the data was tampered with or the key is
// wrong, this returns an error rather than bad data.
func decrypt(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Split the prepended nonce from the ciphertext.
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key or tampered data): %w", err)
	}
	return plaintext, nil
}
