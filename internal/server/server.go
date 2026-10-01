// Package server provides formlander-specific server configuration.
package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"formlander/internal/config"
)

// Build info set at compile time via ldflags
var buildCommit = "dev"

// TemplateFuncs returns formlander-specific template functions.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"safeHTML": func(v interface{}) template.HTML {
			switch val := v.(type) {
			case template.HTML:
				return val
			case string:
				return template.HTML(val)
			default:
				return template.HTML(fmt.Sprint(v))
			}
		},
		"truncateJSON": truncateJSON,
		"who":          who,
		"gist":         gist,
		"ago":          ago,
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

// fields returns the top-level fields of a submission, in a stable order.
func fields(raw string) ([]string, map[string]string) {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return nil, nil
	}
	keys := make([]string, 0, len(payload))
	values := map[string]string{}
	for key, value := range payload {
		text, ok := value.(string)
		if !ok {
			text = strings.Trim(fmt.Sprint(value), "[]")
		}
		if text = strings.Join(strings.Fields(text), " "); text == "" || strings.HasPrefix(key, "_") || key == "cf-turnstile-response" {
			continue
		}
		keys = append(keys, key)
		values[key] = text
	}
	sort.Strings(keys)
	return keys, values
}

// who names the sender of a submission: the email field, or the first value
// that looks like an email, or the name.
func who(raw string) string {
	keys, values := fields(raw)
	for _, key := range []string{"email", "e-mail", "mail", "Email"} {
		if values[key] != "" {
			return values[key]
		}
	}
	for _, key := range keys {
		if strings.Contains(values[key], "@") && !strings.Contains(values[key], " ") {
			return values[key]
		}
	}
	return values["name"]
}

// gist is what a submission says, in one line: the message when there is
// one, or the other fields.
func gist(raw string) string {
	keys, values := fields(raw)
	sender := who(raw)
	text := ""
	for _, key := range []string{"message", "comment", "comments", "body", "use_case", "feedback", "notes", "question"} {
		if values[key] != "" {
			text = values[key]
			break
		}
	}
	if text == "" {
		var rest []string
		for _, key := range keys {
			if values[key] != sender {
				rest = append(rest, key+": "+values[key])
			}
		}
		text = strings.Join(rest, " · ")
	}
	if runes := []rune(text); len(runes) > 110 {
		text = string(runes[:110]) + "…"
	}
	return text
}

// ago says when something happened, the way a person would. It takes a time
// or a pointer to one.
func ago(when any) string {
	var t time.Time
	switch v := when.(type) {
	case time.Time:
		t = v
	case *time.Time:
		if v == nil {
			return ""
		}
		t = *v
	default:
		return ""
	}
	switch d := time.Since(t); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
	return t.Format("Jan 2, 2006")
}
