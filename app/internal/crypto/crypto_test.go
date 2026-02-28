package crypto

import (
	"strings"
	"testing"
)

func init() {
	SetKey([]byte("test-secret-key-at-least-32-bytes!!"))
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	plain := "my-secret-api-token-12345"
	enc, err := Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, encPrefix) {
		t.Fatalf("expected enc:: prefix, got %q", enc[:10])
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plain {
		t.Errorf("round-trip mismatch: got %q, want %q", dec, plain)
	}
}

func TestEncryptDecrypt_Empty(t *testing.T) {
	enc, err := Encrypt("")
	if err != nil || enc != "" {
		t.Errorf("empty encrypt should return empty, got %q err=%v", enc, err)
	}
	dec, err := Decrypt("")
	if err != nil || dec != "" {
		t.Errorf("empty decrypt should return empty, got %q err=%v", dec, err)
	}
}

func TestEncrypt_AlreadyEncrypted(t *testing.T) {
	plain := "token123"
	enc1, _ := Encrypt(plain)
	enc2, _ := Encrypt(enc1)
	if enc1 != enc2 {
		t.Error("encrypting an already-encrypted value should be idempotent")
	}
}

func TestDecrypt_LegacyPlaintext(t *testing.T) {
	legacy := "plaintext-token-no-prefix"
	dec, err := Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt legacy: %v", err)
	}
	if dec != legacy {
		t.Errorf("legacy plaintext should pass through, got %q", dec)
	}
}

func TestEncrypt_UniquePerCall(t *testing.T) {
	plain := "same-token"
	enc1, _ := Encrypt(plain)
	enc2, _ := Encrypt(plain)
	if enc1 == enc2 {
		t.Error("each encryption should produce unique ciphertext (random nonce)")
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"ab", "••••••••"},
		{"abcd", "••••••••"},
		{"abcde", "••••••••bcde"},
		{"my-long-api-token-xyz9", "••••••••xyz9"},
	}
	for _, tt := range tests {
		got := MaskToken(tt.in)
		if got != tt.want {
			t.Errorf("MaskToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDecrypt_BadCiphertext(t *testing.T) {
	_, err := Decrypt(encPrefix + "not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for bad base64")
	}
}

func TestDecrypt_TruncatedData(t *testing.T) {
	_, err := Decrypt(encPrefix + "AAAA")
	if err == nil {
		t.Error("expected error for truncated ciphertext")
	}
}

func TestEncrypt_NoKey(t *testing.T) {
	// Save and clear key
	mu.Lock()
	saved := key
	key = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		key = saved
		mu.Unlock()
	}()

	_, err := Encrypt("test")
	if err == nil {
		t.Error("expected error when key is not set")
	}
}
