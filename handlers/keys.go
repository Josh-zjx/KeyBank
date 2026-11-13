package handlers

import (
	"crypto/ecdh"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"

	"github.com/josh-zjx/keybank/internal/store"
)

const passphraseIterations = 600_000

var keyEncoding = base64.RawURLEncoding.Strict()

func generateKey(kind string) (public, secret []byte, err error) {
	if kind == "aes256" {
		secret = make([]byte, 32)
		_, err = rand.Read(secret)
		return nil, secret, err
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return key.PublicKey().Bytes(), key.Bytes(), nil
}

func protectRecord(rec *store.Record, passphrase string) error {
	if passphrase == "" {
		return nil
	}
	rec.Salt = make([]byte, 16)
	if _, err := rand.Read(rec.Salt); err != nil {
		return err
	}
	rec.Iterations = passphraseIterations
	var err error
	rec.Verifier, err = pbkdf2.Key(sha256.New, passphrase, rec.Salt, rec.Iterations, 32)
	return err
}

// Validate persisted state before authentication or irreversible consumption.
func validateRecord(rec store.Record) error {
	if (rec.Type != "ecc" && rec.Type != "aes256") || len(rec.Secret) != 32 {
		return errors.New("invalid stored key")
	}
	if len(rec.Salt) == 0 && len(rec.Verifier) == 0 && rec.Iterations == 0 {
		return nil
	}
	if len(rec.Salt) != 16 || len(rec.Verifier) != 32 || rec.Iterations != passphraseIterations {
		return errors.New("invalid stored verifier")
	}
	return nil
}

func matchesPassphrase(rec store.Record, passphrase string) bool {
	if len(rec.Verifier) == 0 {
		return true
	}
	verifier, err := pbkdf2.Key(sha256.New, passphrase, rec.Salt, rec.Iterations, 32)
	return err == nil && subtle.ConstantTimeCompare(verifier, rec.Verifier) == 1
}
