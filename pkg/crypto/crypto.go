package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// Encrypt encrypts data using AES-GCM.
// key must be exactly 32 bytes for AES-256.
func Encrypt(key []byte, plaintext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts AES-GCM data.
// key must be exactly 32 bytes.
func Decrypt(key []byte, ciphertext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertextPart := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextPart, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
