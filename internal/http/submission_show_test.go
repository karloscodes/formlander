package http

import (
	"bytes"
	"html/template"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/forms"
	"formlander/internal/server"
	"formlander/web"
)

// The admin layout sets hx-boost on body. htmx 1.9.12 then handles a mailto
// tap on iOS Safari, and the mail app never opens. The reply anchor opts out.
func TestSubmissionShowReplyLinkIsNotBoosted(t *testing.T) {
	tmpl, err := template.New("show").Funcs(server.TemplateFuncs()).ParseFS(web.Templates, "admin/submissions/show.html", "admin/deliveries.html")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "admin/submissions/show/content", map[string]any{
		"Submission": forms.Submission{
			ID:        7,
			DataJSON:  `{"name":"Ada","email":"ada@example.com"}`,
			CreatedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
		},
		"ReplyTo":  "ada@example.com",
		"JSON":     "{}",
		"ReturnTo": "/admin/submissions",
	})
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `<a href="mailto:ada@example.com" hx-boost="false" class="btn-primary">Reply by email</a>`)
}
