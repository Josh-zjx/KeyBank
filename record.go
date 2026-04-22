package main

import "time"

// Record is the value persisted for each generated keypair.
type Record struct {
	PrivPEM   string    `json:"priv_pem"`
	CreatedAt time.Time `json:"created_at"`
}
