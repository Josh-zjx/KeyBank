package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecordJSONRoundTrip(t *testing.T) {
	original := Record{
		PrivPEM:   "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n",
		CreatedAt: time.Now().Truncate(time.Millisecond),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.PrivPEM != original.PrivPEM {
		t.Errorf("PrivPEM: got %q, want %q", got.PrivPEM, original.PrivPEM)
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, original.CreatedAt)
	}
}

func TestRecordZeroValueSafe(t *testing.T) {
	var r Record
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal zero value: %v", err)
	}

	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal zero value: %v", err)
	}
}
