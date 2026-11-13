package store

import "time"

// Record is the immutable payload saved for a one-time key. Secret is either
// an X25519 private key or an AES-256 key. Passphrases are never persisted.
type Record struct {
	Type       string    `json:"type"`
	Secret     []byte    `json:"secret"`
	Salt       []byte    `json:"salt,omitempty"`
	Verifier   []byte    `json:"verifier,omitempty"`
	Iterations int       `json:"iterations,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
