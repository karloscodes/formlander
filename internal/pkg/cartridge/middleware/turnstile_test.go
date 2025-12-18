package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyTurnstileToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req turnstileVerifyRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "test-secret", req.Secret)
		assert.Equal(t, "test-token", req.Response)
		assert.Equal(t, "127.0.0.1", req.RemoteIP)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(turnstileVerifyResponse{
			Success:     true,
			Hostname:    "example.com",
			Action:      "submit",
			ChallengeTS: time.Now().Format(time.RFC3339),
		})
	}))
	defer server.Close()

	// Temporarily override the URL for testing
	originalURL := turnstileVerifyURL
	defer func() { turnstileVerifyURL = originalURL }()
	turnstileVerifyURL = server.URL

	result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "example.com", result.Hostname)
	assert.Equal(t, "submit", result.Action)
}

func TestVerifyTurnstileToken_VerificationFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(turnstileVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-response", "timeout-or-duplicate"},
		})
	}))
	defer server.Close()

	originalURL := turnstileVerifyURL
	defer func() { turnstileVerifyURL = originalURL }()
	turnstileVerifyURL = server.URL

	result, err := VerifyTurnstileToken("test-secret", "invalid-token", "127.0.0.1")
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-input-response")
}

func TestVerifyTurnstileToken_ServerError_Retries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable"))
			return
		}
		// Succeed on third attempt
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(turnstileVerifyResponse{
			Success:  true,
			Hostname: "example.com",
		})
	}))
	defer server.Close()

	originalURL := turnstileVerifyURL
	defer func() { turnstileVerifyURL = originalURL }()
	turnstileVerifyURL = server.URL

	result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 3, attempts, "should have retried twice before succeeding")
}

func TestVerifyTurnstileToken_ClientError_NoRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	originalURL := turnstileVerifyURL
	defer func() { turnstileVerifyURL = originalURL }()
	turnstileVerifyURL = server.URL

	result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
	assert.Equal(t, 1, attempts, "should not retry on 4xx errors")
}

func TestVerifyTurnstileToken_AllRetriesFail(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	originalURL := turnstileVerifyURL
	defer func() { turnstileVerifyURL = originalURL }()
	turnstileVerifyURL = server.URL

	result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTurnstileUnavailable)
	assert.Equal(t, maxRetries, attempts, "should have exhausted all retries")
}
