package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

// turnstileVerifyURL is the Cloudflare Turnstile verification endpoint.
// Variable instead of const to allow testing with mock servers.
var turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Retry configuration
const (
	maxRetries    = 3
	baseBackoff   = 500 * time.Millisecond
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

// TurnstileMiddleware validates Cloudflare Turnstile tokens on form submissions.
// NOTE: This is now a no-op. Turnstile verification is handled per-form via CaptchaProfile.
func TurnstileMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Turnstile verification is now handled per-form in the public controller
		// Individual forms check their captcha_profile_id for captcha requirements

		return c.Next()
	}
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
	client := &http.Client{
		Timeout: requestTimeout,
	}

	resp, err := client.Post(turnstileVerifyURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		// Network errors are retryable
		return nil, fmt.Errorf("%w: %v", ErrTurnstileUnavailable, err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode >= 500 {
		// Server errors are retryable
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrTurnstileUnavailable, resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		// Client errors (4xx) are not retryable
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verification request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var verifyResp turnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
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
