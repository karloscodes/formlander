package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyTurnstileToken(t *testing.T) {
	t.Run("successful verification", func(t *testing.T) {
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

		originalURL := turnstileVerifyURL
		defer func() { turnstileVerifyURL = originalURL }()
		turnstileVerifyURL = server.URL

		result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "example.com", result.Hostname)
		assert.Equal(t, "submit", result.Action)
	})

	t.Run("verification failed with error codes", func(t *testing.T) {
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

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid-input-response")
	})

	t.Run("verification failed without error codes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(turnstileVerifyResponse{
				Success: false,
			})
		}))
		defer server.Close()

		originalURL := turnstileVerifyURL
		defer func() { turnstileVerifyURL = originalURL }()
		turnstileVerifyURL = server.URL

		result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "verification failed")
	})

	t.Run("server error retries and succeeds", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			if count < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("service unavailable"))
				return
			}
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
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, int32(3), attempts, "should have made 3 attempts")
	})

	t.Run("client error does not retry", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
		}))
		defer server.Close()

		originalURL := turnstileVerifyURL
		defer func() { turnstileVerifyURL = originalURL }()
		turnstileVerifyURL = server.URL

		result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status 400")
		assert.Equal(t, int32(1), attempts, "should not have retried")
	})

	t.Run("all retries exhausted", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		originalURL := turnstileVerifyURL
		defer func() { turnstileVerifyURL = originalURL }()
		turnstileVerifyURL = server.URL

		result, err := VerifyTurnstileToken("test-secret", "test-token", "127.0.0.1")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrTurnstileUnavailable)
		assert.Equal(t, int32(maxRetries), attempts, "should have exhausted all retries")
	})
}
