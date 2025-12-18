package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/pkg/cartridge"
	"formlander/internal/pkg/cartridge/middleware"
)

// PublicFormSubmission accepts a submission for the given form slug.
func PublicFormSubmission(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return jsonError(ctx, fiber.StatusInternalServerError, "database unavailable")
	}

	cfg := ctx.Config

	slug := ctx.Params("slug")
	if slug == "" {
		return jsonError(ctx, fiber.StatusNotFound, "form not found")
	}

	form, err := forms.GetBySlug(db, slug)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return jsonError(ctx, fiber.StatusNotFound, "form not found")
		}
		return jsonError(ctx, fiber.StatusInternalServerError, "form lookup failed")
	}

	if token := ctx.Query("token"); token == "" || token != form.Token {
		return jsonError(ctx, fiber.StatusUnauthorized, "invalid token")
	}

	// Check allowed origins (domain allowlisting)
	if !form.IsOriginAllowed(getRequestOrigin(ctx)) {
		return jsonError(ctx, fiber.StatusForbidden, "origin not allowed")
	}

	payload, err := extractSubmissionPayload(ctx, cfg)
	if err != nil {
		// Check for custom error redirect
		if errorURL := extractRedirectURL(payload, "_error_url"); errorURL != "" {
			if err := form.ValidateRedirectURL(errorURL); err == nil {
				return ctx.Redirect(errorURL)
			}
		}
		return jsonError(ctx, fiber.StatusBadRequest, err.Error())
	}

	// Extract custom redirect URLs before saving (don't store them)
	successURL := extractRedirectURL(payload, "_success_url")
	errorURL := extractRedirectURL(payload, "_error_url")

	// Validate redirect URLs
	if successURL != "" {
		if err := form.ValidateRedirectURL(successURL); err != nil {
			return jsonError(ctx, fiber.StatusBadRequest, "invalid success redirect URL")
		}
	}
	if errorURL != "" {
		if err := form.ValidateRedirectURL(errorURL); err != nil {
			return jsonError(ctx, fiber.StatusBadRequest, "invalid error redirect URL")
		}
	}

	if err := enforceCaptchaIfNeeded(ctx, form, payload); err != nil {
		if errorURL != "" {
			return ctx.Redirect(errorURL)
		}
		return jsonError(ctx, fiber.StatusBadRequest, err.Error())
	}

	// Remove special fields from payload
	delete(payload, "_success_url")
	delete(payload, "_error_url")

	logger := ctx.Logger
	userAgent := ctx.Get(fiber.HeaderUserAgent)
	submission, err := forms.CreateSubmission(logger, db, form, payload, userAgent)
	if err != nil {
		// Check for custom error redirect
		if errorURL != "" {
			return ctx.Redirect(errorURL)
		}
		return jsonError(ctx, fiber.StatusInternalServerError, err.Error())
	}

	// Check for custom success redirect
	if successURL != "" {
		return ctx.Redirect(successURL)
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":            true,
		"submission_id": submission.ID,
		"received_at":   submission.CreatedAt.UTC().Format(time.RFC3339),
	})
}

type apiSubmissionRequest struct {
	FormSlug string         `json:"form_slug"`
	Token    string         `json:"token"`
	Data     map[string]any `json:"data"`
}

// APISubmissionCreate handles JSON submissions for external clients.
func APISubmissionCreate(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return jsonError(ctx, fiber.StatusInternalServerError, "database unavailable")
	}

	logger := ctx.Logger

	var req apiSubmissionRequest
	if err := json.Unmarshal(ctx.Body(), &req); err != nil {
		return jsonError(ctx, fiber.StatusBadRequest, "invalid JSON payload")
	}

	if strings.TrimSpace(req.FormSlug) == "" || strings.TrimSpace(req.Token) == "" {
		return jsonError(ctx, fiber.StatusBadRequest, "form_slug and token are required")
	}

	var form forms.Form
	if err := db.Where("slug = ?", req.FormSlug).
		Preload("EmailDelivery").
		Preload("WebhookDelivery").
		Preload("CaptchaProfile").
		First(&form).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return jsonError(ctx, fiber.StatusNotFound, "form not found")
		}
		return jsonError(ctx, fiber.StatusInternalServerError, "form lookup failed")
	}

	if req.Token != form.Token {
		return jsonError(ctx, fiber.StatusUnauthorized, "invalid token")
	}

	// Check allowed origins (domain allowlisting)
	if !form.IsOriginAllowed(getRequestOrigin(ctx)) {
		return jsonError(ctx, fiber.StatusForbidden, "origin not allowed")
	}

	if len(req.Data) == 0 {
		return jsonError(ctx, fiber.StatusBadRequest, "data is required")
	}

	if err := enforceCaptchaIfNeeded(ctx, &form, req.Data); err != nil {
		return jsonError(ctx, fiber.StatusBadRequest, err.Error())
	}

	userAgent := ctx.Get(fiber.HeaderUserAgent)
	submission, err := forms.CreateSubmission(logger, db, &form, req.Data, userAgent)
	if err != nil {
		logger.Error("api submission persistence failed", slog.Any("error", err))
		return jsonError(ctx, fiber.StatusInternalServerError, "failed to save submission")
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":            true,
		"submission_id": submission.ID,
		"received_at":   submission.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func extractSubmissionPayload(ctx *cartridge.Context, cfg *config.Config) (map[string]any, error) {
	result := make(map[string]any)
	fieldCount := 0

	contentType := ctx.Get(fiber.HeaderContentType)
	if strings.Contains(contentType, fiber.MIMEApplicationJSON) {
		if err := json.Unmarshal(ctx.Body(), &result); err != nil {
			return nil, err
		}
	} else {
		if form, err := ctx.MultipartForm(); err == nil && form != nil {
			for key, values := range form.Value {
				fieldCount += len(values)
				if fieldCount > cfg.MaxInputFields {
					return nil, errors.New("too many fields")
				}
				assignFormField(result, key, values)
			}
		} else {
			args := ctx.Request().PostArgs()
			if args == nil {
				return nil, errors.New("submission payload empty")
			}
			args.VisitAll(func(key, value []byte) {
				k := string(key)
				v := string(value)
				fieldCount++
				if fieldCount > cfg.MaxInputFields {
					result["__limit"] = true
					return
				}
				if existing, ok := result[k]; ok {
					switch current := existing.(type) {
					case []string:
						result[k] = append(current, v)
					case string:
						result[k] = []string{current, v}
					}
				} else {
					result[k] = v
				}
			})
			if result["__limit"] == true {
				return nil, errors.New("too many fields")
			}
			delete(result, "__limit")
		}
	}

	if len(result) == 0 {
		return nil, errors.New("submission payload empty")
	}

	return result, nil
}

func assignFormField(dst map[string]any, key string, values []string) {
	if len(values) == 1 {
		dst[key] = values[0]
		return
	}
	array := make([]string, 0, len(values))
	for _, v := range values {
		array = append(array, v)
	}
	dst[key] = array
}

func extractRedirectURL(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if val, ok := payload[key]; ok {
		if str, ok := val.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func enforceCaptchaIfNeeded(ctx *cartridge.Context, form *forms.Form, payload map[string]any) error {
	if form == nil || form.CaptchaProfileID == nil {
		return nil
	}

	logger := ctx.Logger

	if form.CaptchaProfile == nil {
		if logger != nil {
			logger.Warn("captcha profile missing preload", slog.Uint64("form_id", uint64(form.ID)))
		}
		return errors.New("captcha verification failed")
	}

	token, ok := extractCaptchaToken(payload)
	if !ok || strings.TrimSpace(token) == "" {
		return errors.New("captcha verification failed")
	}

	secret := strings.TrimSpace(form.CaptchaProfile.SecretKey)
	if secret == "" {
		if logger != nil {
			logger.Warn("captcha profile missing secret", slog.Uint64("form_id", uint64(form.ID)))
		}
		return errors.New("captcha verification failed")
	}

	result, err := middleware.VerifyTurnstileToken(secret, token, ctx.IP())
	if err != nil {
		if logger != nil {
			logger.Warn("turnstile verification failed",
				slog.Uint64("form_id", uint64(form.ID)),
				slog.Any("error", err),
			)
		}
		return errors.New("captcha verification failed")
	}

	// Validate that the captcha was solved on an allowed origin
	// This prevents token reuse from other sites
	if result.Hostname != "" && strings.TrimSpace(form.AllowedOrigins) != "" && form.AllowedOrigins != "*" {
		if !form.IsOriginAllowed(result.Hostname) {
			if logger != nil {
				logger.Warn("captcha hostname mismatch",
					slog.Uint64("form_id", uint64(form.ID)),
					slog.String("captcha_hostname", result.Hostname),
					slog.String("allowed_origins", form.AllowedOrigins),
				)
			}
			return errors.New("captcha verification failed")
		}
	}

	return nil
}

func extractCaptchaToken(payload map[string]any) (string, bool) {
	if payload == nil {
		return "", false
	}
	keys := []string{"cf-turnstile-response", "cf_turnstile_response"}
	for _, key := range keys {
		if raw, ok := payload[key]; ok {
			delete(payload, key)
			if token := coerceToString(raw); token != "" {
				return token, true
			}
		}
	}
	return "", false
}

func coerceToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	case []byte:
		return string(v)
	}
	return ""
}

func jsonError(ctx *cartridge.Context, status int, message string) error {
	return ctx.Status(status).JSON(fiber.Map{
		"ok":    false,
		"error": message,
	})
}

// getRequestOrigin extracts the origin domain from the request headers.
// Checks Origin header first, falls back to Referer.
// Returns an extracted domain (e.g., "example.com"), not the full URL.
func getRequestOrigin(ctx *cartridge.Context) string {
	origin := ctx.Get("Origin")
	if origin == "" {
		origin = ctx.Get("Referer")
	}
	if origin == "" {
		return ""
	}
	return extractDomain(origin)
}

// extractDomain extracts the domain from a full URL (origin or referer).
// Removes protocol, path, query, fragment, and port.
func extractDomain(urlStr string) string {
	// Remove protocol
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")

	// Remove path, query and fragment
	if idx := strings.IndexAny(urlStr, "/?#"); idx >= 0 {
		urlStr = urlStr[:idx]
	}

	// Remove port
	if idx := strings.LastIndex(urlStr, ":"); idx >= 0 {
		urlStr = urlStr[:idx]
	}

	return strings.ToLower(urlStr)
}
