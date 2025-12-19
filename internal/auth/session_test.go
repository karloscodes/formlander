package auth

import (
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	t.Run("generates hash from password", func(t *testing.T) {
		password := "test-password-123"

		hash, err := GeneratePasswordHash(password)
		if err != nil {
			t.Fatalf("GeneratePasswordHash() error = %v", err)
		}

		if len(hash) == 0 {
			t.Error("GeneratePasswordHash() returned empty hash")
		}

		// Hash should be different each time (bcrypt includes salt)
		hash2, err := GeneratePasswordHash(password)
		if err != nil {
			t.Fatalf("GeneratePasswordHash() second call error = %v", err)
		}

		if string(hash) == string(hash2) {
			t.Error("GeneratePasswordHash() should return different hashes due to salt")
		}
	})
}

func TestVerifyPassword(t *testing.T) {
	password := "correct-password"
	hash, err := GeneratePasswordHash(password)
	if err != nil {
		t.Fatalf("GeneratePasswordHash() error = %v", err)
	}

	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"correct password", "correct-password", true},
		{"wrong password", "wrong-password", false},
		{"empty password", "", false},
		{"case sensitive", "Correct-Password", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyPassword(string(hash), tt.password)
			if got != tt.want {
				t.Errorf("VerifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}
