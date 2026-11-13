package store

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestRecordJSONRoundTrip(t *testing.T) {
	original := Record{
		Secret:    []byte("0123456789abcdef0123456789abcdef"),
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

	if !bytes.Equal(got.Secret, original.Secret) {
		t.Errorf("Secret: got %q, want %q", got.Secret, original.Secret)
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
