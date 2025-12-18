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
	tests := []struct {
		name            string
		secret          string
		token           string
		remoteIP        string
		serverHandler   func(t *testing.T, attempts *int32) http.HandlerFunc
		expectedResult  *TurnstileResult
		expectError     bool
		errorContains   string
		errorIs         error
		expectedAttempts int32
	}{
		{
			name:     "successful verification",
			secret:   "test-secret",
			token:    "test-token",
			remoteIP: "127.0.0.1",
			serverHandler: func(t *testing.T, attempts *int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(attempts, 1)
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
				}
			},
			expectedResult:   &TurnstileResult{Success: true, Hostname: "example.com", Action: "submit"},
			expectError:      false,
			expectedAttempts: 1,
		},
		{
			name:     "verification failed with error codes",
			secret:   "test-secret",
			token:    "invalid-token",
			remoteIP: "127.0.0.1",
			serverHandler: func(t *testing.T, attempts *int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(attempts, 1)
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(turnstileVerifyResponse{
						Success:    false,
						ErrorCodes: []string{"invalid-input-response", "timeout-or-duplicate"},
					})
				}
			},
			expectedResult:   nil,
			expectError:      true,
			errorContains:    "invalid-input-response",
			expectedAttempts: 1,
		},
		{
			name:     "server error retries and succeeds",
			secret:   "test-secret",
			token:    "test-token",
			remoteIP: "127.0.0.1",
			serverHandler: func(t *testing.T, attempts *int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					count := atomic.AddInt32(attempts, 1)
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
				}
			},
			expectedResult:   &TurnstileResult{Success: true, Hostname: "example.com"},
			expectError:      false,
			expectedAttempts: 3,
		},
		{
			name:     "client error does not retry",
			secret:   "test-secret",
			token:    "test-token",
			remoteIP: "127.0.0.1",
			serverHandler: func(t *testing.T, attempts *int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(attempts, 1)
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("bad request"))
				}
			},
			expectedResult:   nil,
			expectError:      true,
			errorContains:    "status 400",
			expectedAttempts: 1,
		},
		{
			name:     "all retries exhausted",
			secret:   "test-secret",
			token:    "test-token",
			remoteIP: "127.0.0.1",
			serverHandler: func(t *testing.T, attempts *int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(attempts, 1)
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("internal error"))
				}
			},
			expectedResult:   nil,
			expectError:      true,
			errorIs:         ErrTurnstileUnavailable,
			expectedAttempts: int32(maxRetries),
		},
		{
			name:     "verification failed without error codes",
			secret:   "test-secret",
			token:    "test-token",
			remoteIP: "127.0.0.1",
			serverHandler: func(t *testing.T, attempts *int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(attempts, 1)
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(turnstileVerifyResponse{
						Success: false,
					})
				}
			},
			expectedResult:   nil,
			expectError:      true,
			errorContains:    "verification failed",
			expectedAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			server := httptest.NewServer(tt.serverHandler(t, &attempts))
			defer server.Close()

			// Override URL for testing
			originalURL := turnstileVerifyURL
			defer func() { turnstileVerifyURL = originalURL }()
			turnstileVerifyURL = server.URL

			result, err := VerifyTurnstileToken(tt.secret, tt.token, tt.remoteIP)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Success, result.Success)
				assert.Equal(t, tt.expectedResult.Hostname, result.Hostname)
				assert.Equal(t, tt.expectedResult.Action, result.Action)
			}

			assert.Equal(t, tt.expectedAttempts, attempts, "unexpected number of attempts")
		})
	}
}
