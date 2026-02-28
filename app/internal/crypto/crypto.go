package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
)

const encPrefix = "enc::" // prefix that marks an already-encrypted value

var (
	mu  sync.RWMutex
	key []byte // 32-byte AES-256 key derived from auth secret
)

// SetKey derives and stores the AES-256 encryption key from the auth secret.
// Must be called before Encrypt/Decrypt (called once at startup and after setup).
func SetKey(authSecret []byte) {
	h := sha256.Sum256(authSecret) // always exactly 32 bytes
	mu.Lock()
	key = h[:]
	mu.Unlock()
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns "enc::<base64>" or empty string if input is empty.
// If the value is already encrypted (has the prefix), it is returned as-is.
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.HasPrefix(plaintext, encPrefix) {
		return plaintext, nil // already encrypted
	}

	mu.RLock()
	k := key
	mu.RUnlock()
	if len(k) == 0 {
		return "", errors.New("crypto: encryption key not set")
	}

	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts an "enc::<base64>" value back to plaintext.
// If the value has no prefix (legacy plaintext), it is returned as-is.
func Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	if !strings.HasPrefix(encrypted, encPrefix) {
		return encrypted, nil // legacy plaintext — will be encrypted on next save
	}

	mu.RLock()
	k := key
	mu.RUnlock()
	if len(k) == 0 {
		return "", errors.New("crypto: encryption key not set")
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, encPrefix))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", errors.New("crypto: ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// MaskToken returns a masked version of a token for display.
// Shows only the last 4 characters, e.g. "••••••••abc1"
func MaskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return strings.Repeat("•", 8)
	}
	return strings.Repeat("•", 8) + token[len(token)-4:]
}
