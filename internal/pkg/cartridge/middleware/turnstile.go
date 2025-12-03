package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

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
// Exported for use in form submission controllers.
func VerifyTurnstileToken(secret, token, remoteIP string) error {
	reqBody := turnstileVerifyRequest{
		Secret:   secret,
		Response: token,
		RemoteIP: remoteIP,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(turnstileVerifyURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to verify token: %w", err)
	}
	defer resp.Body.Close()

	var verifyResp turnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !verifyResp.Success {
		if len(verifyResp.ErrorCodes) > 0 {
			return fmt.Errorf("verification failed with errors: %v", verifyResp.ErrorCodes)
		}
		return fmt.Errorf("verification failed")
	}

	return nil
}
