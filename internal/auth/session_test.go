package auth

import (
	"encoding/json"
	"testing"
	"time"

	"formlander/internal/config"
)

func TestGeneratePasswordHash(t *testing.T) {
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
		{
			name:     "correct password",
			password: "correct-password",
			want:     true,
		},
		{
			name:     "wrong password",
			password: "wrong-password",
			want:     false,
		},
		{
			name:     "empty password",
			password: "",
			want:     false,
		},
		{
			name:     "case sensitive",
			password: "Correct-Password",
			want:     false,
		},
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

func TestInitialize(t *testing.T) {
	cfg := &config.Config{
		SessionSecret:         "test-secret-key-for-signing",
		SessionTimeoutSeconds: 3600,
		Environment:           "production",
	}
	
	Initialize(cfg)
	
	if string(sessionSecret) != cfg.SessionSecret {
		t.Errorf("sessionSecret = %v, want %v", string(sessionSecret), cfg.SessionSecret)
	}
	
	expectedTTL := time.Duration(3600) * time.Second
	if sessionTTL != expectedTTL {
		t.Errorf("sessionTTL = %v, want %v", sessionTTL, expectedTTL)
	}
	
	if !isProduction {
		t.Error("isProduction should be true for production environment")
	}
}

func TestSignAndVerify(t *testing.T) {
	// Initialize with test secret
	Initialize(&config.Config{
		SessionSecret:         "test-secret-for-hmac-signing",
		SessionTimeoutSeconds: 3600,
		Environment:           "development",
	})
	
	tests := []struct {
		name      string
		sessionData SessionData
	}{
		{
			name: "valid session",
			sessionData: SessionData{
				UserID:    "123",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
		},
		{
			name: "different user",
			sessionData: SessionData{
				UserID:    "999",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal session data
			jsonData, err := json.Marshal(tt.sessionData)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			
			// Sign the data
			token, err := sign(jsonData)
			if err != nil {
				t.Fatalf("sign() error = %v", err)
			}
			
			if token == "" {
				t.Error("sign() returned empty token")
			}
			
			// Verify the token
			verifiedData, err := verify(token)
			if err != nil {
				t.Fatalf("verify() error = %v", err)
			}
			
			if verifiedData.UserID != tt.sessionData.UserID {
				t.Errorf("UserID = %v, want %v", verifiedData.UserID, tt.sessionData.UserID)
			}
			
			// Times should be close (within 1 second due to marshaling)
			timeDiff := verifiedData.ExpiresAt.Sub(tt.sessionData.ExpiresAt)
			if timeDiff > time.Second || timeDiff < -time.Second {
				t.Errorf("ExpiresAt diff = %v, want within 1 second", timeDiff)
			}
		})
	}
}

func TestVerifyInvalidTokens(t *testing.T) {
	Initialize(&config.Config{
		SessionSecret:         "test-secret-key",
		SessionTimeoutSeconds: 3600,
		Environment:           "development",
	})
	
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "malformed token (no dot)",
			token: "invalidtoken",
		},
		{
			name:  "invalid base64 payload",
			token: "!!!invalid!!!.signature",
		},
		{
			name:  "invalid base64 signature",
			token: "eyJ1c2VySWQiOiIxMjMifQ.!!!invalid!!!",
		},
		{
			name:  "tampered payload",
			token: "eyJ1c2VySWQiOiI5OTkifQ.abc123def456",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verify(tt.token)
			if err == nil {
				t.Error("verify() should return error for invalid token")
			}
		})
	}
}

func TestSignatureVerification(t *testing.T) {
	Initialize(&config.Config{
		SessionSecret:         "secret1",
		SessionTimeoutSeconds: 3600,
		Environment:           "development",
	})
	
	sessionData := SessionData{
		UserID:    "123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	
	jsonData, _ := json.Marshal(sessionData)
	token, _ := sign(jsonData)
	
	// Change the secret
	Initialize(&config.Config{
		SessionSecret:         "secret2",
		SessionTimeoutSeconds: 3600,
		Environment:           "development",
	})
	
	// Verification should fail with different secret
	_, err := verify(token)
	if err == nil {
		t.Error("verify() should fail when secret is changed")
	}
}

func TestComputeHMAC(t *testing.T) {
	payload := []byte("test payload")
	secret := []byte("test secret")
	
	mac1 := computeHMAC(payload, secret)
	mac2 := computeHMAC(payload, secret)
	
	// Same input should produce same output
	if string(mac1) != string(mac2) {
		t.Error("computeHMAC() should be deterministic")
	}
	
	// Different payload should produce different output
	mac3 := computeHMAC([]byte("different payload"), secret)
	if string(mac1) == string(mac3) {
		t.Error("computeHMAC() should produce different output for different payload")
	}
	
	// Different secret should produce different output
	mac4 := computeHMAC(payload, []byte("different secret"))
	if string(mac1) == string(mac4) {
		t.Error("computeHMAC() should produce different output for different secret")
	}
}

func TestSessionDataMarshaling(t *testing.T) {
	sessionData := SessionData{
		UserID:    "456",
		ExpiresAt: time.Now().Add(2 * time.Hour).Truncate(time.Second),
	}
	
	jsonData, err := json.Marshal(sessionData)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	var unmarshaled SessionData
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	if unmarshaled.UserID != sessionData.UserID {
		t.Errorf("UserID = %v, want %v", unmarshaled.UserID, sessionData.UserID)
	}
	
	// Compare timestamps (truncate to avoid precision issues)
	if !unmarshaled.ExpiresAt.Truncate(time.Second).Equal(sessionData.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", unmarshaled.ExpiresAt, sessionData.ExpiresAt)
	}
}
