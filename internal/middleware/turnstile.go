package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// turnstileVerifyURL is the Cloudflare Turnstile verification endpoint.
// Variable instead of const to allow testing with mock servers.
var turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Retry configuration
const (
	maxRetries     = 3
	baseBackoff    = 500 * time.Millisecond
	requestTimeout = 10 * time.Second
)

// ErrTurnstileUnavailable indicates Cloudflare API is temporarily unavailable
var ErrTurnstileUnavailable = errors.New("turnstile service temporarily unavailable")

type turnstileVerifyRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
	RemoteIP string `json:"remoteip,omitempty"`
}

type turnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
	Action      string   `json:"action,omitempty"`
	CData       string   `json:"cdata,omitempty"`
}

// TurnstileResult contains the verification result including metadata for validation
type TurnstileResult struct {
	Success  bool
	Hostname string
	Action   string
}

// VerifyTurnstileToken validates a Cloudflare Turnstile token.
// Returns the verification result including hostname for origin validation.
// Retries on transient errors (5xx, network issues) with exponential backoff.
func VerifyTurnstileToken(secret, token, remoteIP string) (*TurnstileResult, error) {
	reqBody := turnstileVerifyRequest{
		Secret:   secret,
		Response: token,
		RemoteIP: remoteIP,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s, 2s
			backoff := baseBackoff * time.Duration(1<<(attempt-1))
			time.Sleep(backoff)
		}

		result, err := doVerifyRequest(jsonData)
		if err == nil {
			return result, nil
		}

		// Check if error is retryable
		if errors.Is(err, ErrTurnstileUnavailable) {
			lastErr = err
			continue
		}

		// Non-retryable error (4xx, verification failed, etc.)
		return nil, err
	}

	return nil, fmt.Errorf("%w: %v", ErrTurnstileUnavailable, lastErr)
}

func doVerifyRequest(jsonData []byte) (*TurnstileResult, error) {
	status, body, err := postVerify(jsonData)
	if err != nil {
		return nil, err
	}

	if status != http.StatusOK {
		// Client errors (4xx) are not retryable
		return nil, fmt.Errorf("verification request failed with status %d: %s", status, string(body))
	}

	var verifyResp turnstileVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !verifyResp.Success {
		if len(verifyResp.ErrorCodes) > 0 {
			return nil, fmt.Errorf("verification failed: %v", verifyResp.ErrorCodes)
		}
		return nil, errors.New("verification failed")
	}

	return &TurnstileResult{
		Success:  true,
		Hostname: verifyResp.Hostname,
		Action:   verifyResp.Action,
	}, nil
}

// postVerify sends one request to the verification endpoint and returns the
// status and the body of the answer. A network error and a server error (5xx)
// are ErrTurnstileUnavailable: a retry can help.
func postVerify(jsonData []byte) (status int, body []byte, err error) {
	client := &http.Client{
		Timeout: requestTimeout,
	}

	resp, err := client.Post(turnstileVerifyURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrTurnstileUnavailable, err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return resp.StatusCode, body, fmt.Errorf("%w: status %d: %s", ErrTurnstileUnavailable, resp.StatusCode, string(body))
	}
	return resp.StatusCode, body, nil
}

// CheckTurnstileSecret asks Cloudflare whether it knows a secret key. Nobody
// can solve a captcha on the server, so the request carries a token that is
// not real. Cloudflare then names what it rejects: the secret
// (invalid-input-secret, with status 400) or only the token
// (invalid-input-response). The second answer means the secret is good.
func CheckTurnstileSecret(secret string) (accepted bool, err error) {
	jsonData, err := json.Marshal(turnstileVerifyRequest{Secret: secret, Response: "formlander-secret-check"})
	if err != nil {
		return false, err
	}

	status, body, err := postVerify(jsonData)
	if err != nil {
		return false, err
	}
	var verifyResp turnstileVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return false, fmt.Errorf("Cloudflare answered with status %d and no JSON", status)
	}
	for _, code := range verifyResp.ErrorCodes {
		if code == "invalid-input-secret" || code == "missing-input-secret" {
			return false, nil
		}
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("Cloudflare answered with status %d: %v", status, verifyResp.ErrorCodes)
	}
	return true, nil
}
