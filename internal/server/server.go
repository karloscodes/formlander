// Package server provides formlander-specific server configuration.
package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"formlander/internal/config"
)

// Build info set at compile time via ldflags
var buildCommit = "dev"

// TemplateFuncs returns formlander-specific template functions.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"truncateJSON": truncateJSON,
		"assetVersion": func() string {
			if buildCommit == "dev" {
				return time.Now().Format("20060102150405")
			}
			if len(buildCommit) > 8 {
				return buildCommit[:8]
			}
			return buildCommit
		},
	}
}

// ErrorHandler returns formlander-specific error handler.
func ErrorHandler(log *slog.Logger, cfg *config.Config) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		log.Error("request failed",
			slog.Any("error", err),
			slog.String("path", c.Path()),
			slog.String("method", c.Method()),
			slog.Int("status", code),
		)

		// JSON error response for API requests
		if c.Accepts(fiber.MIMEApplicationJSON) == fiber.MIMEApplicationJSON {
			return c.Status(code).JSON(fiber.Map{
				"error":   "internal_server_error",
				"message": err.Error(),
			})
		}

		// HTML error page for browser requests
		if code == fiber.StatusInternalServerError {
			return c.Status(code).Render("layouts/base", fiber.Map{
				"Title":             "500 - Internal Server Error",
				"ContentView":       "errors/500/content",
				"DevMode":           cfg.IsDevelopment(),
				"ErrorMessage":      err.Error(),
				"HideHeaderActions": true,
			}, "")
		}

		return c.Status(code).SendString(fmt.Sprintf("Error: %d - %s", code, err.Error()))
	}
}

func truncateJSON(raw string) string {
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if canonical, err := json.Marshal(payload); err == nil {
			raw = string(canonical)
		}
	}
	const limit = 80
	if len(raw) <= limit {
		return raw
	}
	return raw[:limit] + "..."
}
