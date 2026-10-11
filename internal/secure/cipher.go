package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// Cipher uses an operator-managed key. Associated data binds ciphertext to its tenant and record.
type Cipher struct{ aead cipher.AEAD }

func New(key string) (*Cipher, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(b) != 32 {
		return nil, errors.New("master key must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(b)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{a}, nil
}
func (c *Cipher) Seal(plain, binding string) (string, error) {
	if c == nil {
		return "", errors.New("encryption is not configured")
	}
	n := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, n); err != nil {
		return "", err
	}
	return "enc1:" + base64.StdEncoding.EncodeToString(c.aead.Seal(n, n, []byte(plain), []byte(binding))), nil
}
func (c *Cipher) Open(value, binding string) (string, error) {
	if c == nil || !strings.HasPrefix(value, "enc1:") {
		return "", errors.New("encrypted record required")
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "enc1:"))
	if err != nil || len(b) < c.aead.NonceSize() {
		return "", errors.New("invalid encrypted record")
	}
	n := c.aead.NonceSize()
	p, err := c.aead.Open(nil, b[:n], b[n:], []byte(binding))
	if err != nil {
		return "", errors.New("record authentication failed")
	}
	return string(p), nil
}
