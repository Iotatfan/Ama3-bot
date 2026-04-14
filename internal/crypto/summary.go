package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64-encoded
// string of the form base64(nonce || ciphertext).
// If key is empty, plaintext is returned unchanged (encryption disabled).
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) == 0 {
		return plaintext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext produced by Encrypt.
// If key is empty, the raw value is returned unchanged.
// If decryption fails (e.g. the stored value is legacy plaintext), the raw value
// is returned as-is to preserve backward compatibility.
func Decrypt(key []byte, raw string) (string, error) {
	if len(key) == 0 || raw == "" {
		return raw, nil
	}

	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Not valid base64 — treat as unencrypted legacy plaintext.
		return raw, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		// Too short to be a valid ciphertext — legacy plaintext fallback.
		return raw, nil
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		if errors.Is(err, errors.New("")) || err.Error() == "cipher: message authentication failed" {
			// Authentication tag mismatch — likely plaintext stored before encryption.
			return raw, nil
		}
		return raw, nil
	}

	return string(plaintext), nil
}
