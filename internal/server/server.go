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
		"details":      details,
		"fileSize":     fileSize,
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

		// A program gets JSON. A browser asks for HTML first, and gets a page.
		if c.Accepts(fiber.MIMEApplicationJSON, fiber.MIMETextHTML) != fiber.MIMETextHTML {
			return c.Status(code).JSON(fiber.Map{
				"error":   "internal_server_error",
				"message": err.Error(),
			})
		}

		heading, message := "Something went wrong", "Formlander could not finish this request. Try again. The server log has the details."
		switch code {
		case fiber.StatusNotFound:
			heading, message = "Page not found", "This page does not exist, or it was deleted."
		case fiber.StatusForbidden, fiber.StatusUnauthorized:
			heading, message = "Not allowed", "You cannot open this page."
		}
		data := fiber.Map{
			"Title":             heading,
			"ContentView":       "errors/500/content",
			"Code":              code,
			"Heading":           heading,
			"Message":           message,
			"HideHeaderActions": true,
		}
		if cfg.IsDevelopment() && code >= fiber.StatusInternalServerError {
			data["ErrorMessage"] = err.Error()
		}
		return c.Status(code).Render("layouts/base", data, "")
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

// Detail is one field of a submission: what the form called it and what the
// person typed.
type Detail struct {
	Label string
	Value string
}

// details returns every field of a submission as a label and a value, in a
// stable order, for the page of one submission. It returns nil when the
// submission is not a JSON object.
func details(raw string) []Detail {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return nil
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		if key != "cf-turnstile-response" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	list := make([]Detail, 0, len(keys))
	for _, key := range keys {
		list = append(list, Detail{Label: label(key), Value: plain(payload[key])})
	}
	return list
}

// label turns the name of a form field into words: "use_case" is "Use case".
func label(key string) string {
	words := strings.Join(strings.FieldsFunc(key, func(r rune) bool { return r == '_' || r == '-' }), " ")
	if words == "" {
		return key
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

// plain writes a value of a submission the way a person reads it: text as
// it is, a list with commas, and anything deeper as JSON.
func plain(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = plain(item)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		encoded, _ := json.MarshalIndent(v, "", "  ")
		return string(encoded)
	}
	return fmt.Sprint(value)
}

// fileSize writes a number of bytes the way a person reads it.
func fileSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%d KB", bytes>>10)
	}
	return fmt.Sprintf("%d bytes", bytes)
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
