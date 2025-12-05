package controllers

import (
	"encoding/json"
	"errors"
	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/pkg/cartridge"
	"formlander/internal/pkg/cartridge/middleware"
	urlpkg "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type submissionWithPreview struct {
	forms.Submission
	DataJSONPreview string
}

// SubmissionList shows all submissions with pagination and filters.
func SubmissionList(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	// Parse pagination
	page, _ := strconv.Atoi(ctx.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage := 20
	offset := (page - 1) * perPage

	// Parse filters
	formID := ctx.Query("form_id")
	rangeFilter := ctx.Query("range")

	// Build query
	query := db.Model(&forms.Submission{}).Preload("Form")

	if formID != "" {
		query = query.Where("form_id = ?", formID)
	}

	// Handle date range filter
	if rangeFilter != "" && rangeFilter != "all" {
		var startTime time.Time
		now := time.Now()
		switch rangeFilter {
		case "7d":
			startTime = now.AddDate(0, 0, -7)
		case "30d":
			startTime = now.AddDate(0, 0, -30)
		case "90d":
			startTime = now.AddDate(0, 0, -90)
		}
		if !startTime.IsZero() {
			query = query.Where("created_at >= ?", startTime)
		}
	}

	// Get total count for pagination
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return fiber.ErrInternalServerError
	}

	// Get submissions for current page
	var submissions []forms.Submission
	if err := query.Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&submissions).Error; err != nil {
		return fiber.ErrInternalServerError
	}

	// Add previews
	submissionsWithPreview := make([]submissionWithPreview, len(submissions))
	for i, sub := range submissions {
		preview := sub.DataJSON
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		// Clean up for display
		preview = strings.ReplaceAll(preview, "\n", " ")
		submissionsWithPreview[i] = submissionWithPreview{
			Submission:      sub,
			DataJSONPreview: preview,
		}
	}

	// Get all forms for filter dropdown
	var forms []forms.Form
	db.Select("id, name").Order("name ASC").Find(&forms)

	// Calculate pagination info
	totalPages := (int(totalCount) + perPage - 1) / perPage
	hasNext := page < totalPages
	hasPrev := page > 1
	nextPage := page + 1
	prevPage := page - 1

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Submissions",
		"Submissions": submissionsWithPreview,
		"Forms":       forms,
		"Page":        page,
		"NextPage":    nextPage,
		"PrevPage":    prevPage,
		"TotalPages":  totalPages,
		"TotalCount":  totalCount,
		"HasNext":     hasNext,
		"HasPrev":     hasPrev,
		"FormID":      formID,
		"Range":       rangeFilter,
		"ContentView": "admin/submissions/index/content",
	}, "")
}

// AdminSubmissionShow renders a single submission payload.
func AdminSubmissionShow(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	var submission forms.Submission
	if err := db.Preload("Form").Preload("WebhookEvents").Preload("EmailEvents").Where("id = ?", id).First(&submission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	var prettyJSON string
	if submission.DataJSON != "" {
		var buf any
		if err := json.Unmarshal([]byte(submission.DataJSON), &buf); err == nil {
			formatted, _ := json.MarshalIndent(buf, "", "  ")
			prettyJSON = string(formatted)
		} else {
			prettyJSON = submission.DataJSON
		}
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Submission",
		"Submission":  submission,
		"JSON":        prettyJSON,
		"ContentView": "admin/submissions/show/content",
	}, "")
}

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
	if !isOriginAllowed(ctx, form) {
		return jsonError(ctx, fiber.StatusForbidden, "origin not allowed")
	}

	payload, err := extractSubmissionPayload(ctx, cfg)
	if err != nil {
		// Check for custom error redirect
		if errorURL := extractRedirectURL(payload, "_error_url"); errorURL != "" {
			if err := validateRedirectURL(errorURL, form); err == nil {
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
		if err := validateRedirectURL(successURL, form); err != nil {
			return jsonError(ctx, fiber.StatusBadRequest, "invalid success redirect URL")
		}
	}
	if errorURL != "" {
		if err := validateRedirectURL(errorURL, form); err != nil {
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
	if !isOriginAllowed(ctx, &form) {
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
		logger.Error("api submission persistence failed", zap.Error(err))
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

func validateRedirectURL(url string, form *forms.Form) error {
	if url == "" {
		return nil // No redirect is fine
	}

	parsed, err := urlpkg.Parse(url)
	if err != nil {
		return errors.New("invalid redirect URL")
	}

	// Allow relative URLs (no host)
	if parsed.Host == "" {
		return nil
	}

	// Check against allowed origins for this form
	if form.AllowedOrigins == "" {
		return errors.New("absolute redirects not allowed without configured origins")
	}

	// Parse allowed origins
	allowedOrigins := strings.Split(form.AllowedOrigins, "\n")
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}

		allowedURL, err := urlpkg.Parse(origin)
		if err != nil {
			continue
		}

		// Match exact host or subdomain
		if parsed.Host == allowedURL.Host || strings.HasSuffix(parsed.Host, "."+allowedURL.Host) {
			return nil
		}
	}

	return errors.New("redirect URL not in allowed origins")
}

func enforceCaptchaIfNeeded(ctx *cartridge.Context, form *forms.Form, payload map[string]any) error {
	if form == nil || form.CaptchaProfileID == nil {
		return nil
	}

	if form.CaptchaProfile == nil {
		logger := ctx.Logger
		if logger != nil {
			logger.Warn("captcha profile missing preload", zap.Uint("form_id", form.ID))
		}
		return errors.New("captcha verification failed")
	}

	token, ok := extractCaptchaToken(payload)
	if !ok || strings.TrimSpace(token) == "" {
		return errors.New("captcha verification failed")
	}

	secret := strings.TrimSpace(form.CaptchaProfile.SecretKey)
	if secret == "" {
		logger := ctx.Logger
		if logger != nil {
			logger.Warn("captcha profile missing secret", zap.Uint("form_id", form.ID))
		}
		return errors.New("captcha verification failed")
	}

	if err := middleware.VerifyTurnstileToken(secret, token, ctx.IP()); err != nil {
		logger := ctx.Logger
		if logger != nil {
			logger.Warn("turnstile verification failed", zap.Uint("form_id", form.ID), zap.Error(err))
		}
		return errors.New("captcha verification failed")
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

// isOriginAllowed checks if the request origin/referer is allowed for this form.
// If AllowedOrigins is empty, all origins are allowed (backwards compatible).
// If AllowedOrigins is "*", all origins are allowed.
// Otherwise, the origin must match one of the allowed domains.
func isOriginAllowed(ctx *cartridge.Context, form *forms.Form) bool {
	// If no restrictions configured, allow all
	allowedOrigins := strings.TrimSpace(form.AllowedOrigins)
	if allowedOrigins == "" || allowedOrigins == "*" {
		return true
	}

	// Get origin from Origin or Referer header
	origin := ctx.Get("Origin")
	if origin == "" {
		origin = ctx.Get("Referer")
	}
	if origin == "" {
		// No origin header - could be direct API call or cURL
		// For security, reject if origins are configured
		return false
	}

	// Parse the origin to get the domain
	originDomain := extractDomain(origin)
	if originDomain == "" {
		return false
	}

	// Check against allowed origins (comma-separated list)
	allowedList := strings.Split(allowedOrigins, ",")
	for _, allowed := range allowedList {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}

		// Wildcard - allow all
		if allowed == "*" {
			return true
		}

		// Exact match or wildcard subdomain match
		if originDomain == allowed || strings.HasSuffix(originDomain, "."+allowed) {
			return true
		}

		// Support wildcard patterns like *.example.com
		if strings.HasPrefix(allowed, "*.") {
			baseDomain := strings.TrimPrefix(allowed, "*.")
			if originDomain == baseDomain || strings.HasSuffix(originDomain, "."+baseDomain) {
				return true
			}
		}
	}

	return false
}

// extractDomain extracts the domain from a full URL (origin or referer)
func extractDomain(urlStr string) string {
	// Remove protocol
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")

	// Remove path and query
	if idx := strings.IndexAny(urlStr, "/?#"); idx >= 0 {
		urlStr = urlStr[:idx]
	}

	// Remove port
	if idx := strings.LastIndex(urlStr, ":"); idx >= 0 {
		urlStr = urlStr[:idx]
	}

	return strings.ToLower(urlStr)
}
