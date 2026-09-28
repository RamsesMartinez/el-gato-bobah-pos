package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// Local cifra con AES-256-GCM y una llave de la máquina. Es el respaldo de DESARROLLO: conserva el
// formato, la AAD y el «ilegible» de otro ambiente, y pierde lo que solo existe con Google (permisos
// de la cuenta de servicio, red). Esa parte se prueba en el ambiente de pruebas, que usa KMS real.
type Local struct{ aead cipher.AEAD }

// NewLocal exige 32 bytes: AES-256, sin adivinar un tamaño menor.
func NewLocal(key []byte) (*Local, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets: the local key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Local{aead: aead}, nil
}

// Encrypt devuelve formato || nonce || sellado.
func (l *Local) Encrypt(_ context.Context, plaintext []byte, aad string) ([]byte, error) {
	nonce := make([]byte, l.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{formatLocal}, nonce...)
	return l.aead.Seal(out, nonce, plaintext, []byte(aad)), nil
}

func (l *Local) Decrypt(_ context.Context, ciphertext []byte, aad string) ([]byte, error) {
	n := l.aead.NonceSize()
	if len(ciphertext) < 1+n+l.aead.Overhead() || ciphertext[0] != formatLocal {
		return nil, ErrUnreadable
	}
	plaintext, err := l.aead.Open(nil, ciphertext[1:1+n], ciphertext[1+n:], []byte(aad))
	if err != nil {
		return nil, ErrUnreadable
	}
	return plaintext, nil
}
