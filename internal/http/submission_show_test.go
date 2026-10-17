package http

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/forms"
	"formlander/internal/server"
	"formlander/web"
)

// Reply by email renders a real mailto URL. html/template leaves this address
// alone: it is not replaced with #ZgotmplZ and the @ is not percent-encoded.
// The anchor also opts out of hx-boost. On iOS Safari a mailto link's
// hostname is the page host, so htmx 1.9.12 would cancel the tap.
func TestSubmissionShowReplyHref(t *testing.T) {
	tmpl := parseTemplates(t, "admin/submissions/show.html", "admin/deliveries.html")

	address := "trongnhanyb38@gmail.com"
	submission := forms.Submission{
		ID:        7,
		DataJSON:  `{"name":"Carlos","email":"` + address + `"}`,
		CreatedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
	}
	require.Equal(t, address, submission.ReplyAddress())

	html := render(t, tmpl, "admin/submissions/show/content", map[string]any{
		"Submission": submission,
		"ReplyTo":    submission.ReplyAddress(),
		"JSON":       "{}",
		"ReturnTo":   "/admin/submissions",
	})

	const prefix = `href="mailto:`
	i := strings.Index(html, prefix)
	require.NotEqual(t, -1, i)
	href, _, _ := strings.Cut(html[i+len(`href="`):], `"`)
	t.Logf("rendered href: %s", href)

	assert.Equal(t, "mailto:"+address, href)
	assert.Contains(t, html, `<a href="mailto:`+address+`" hx-boost="false" class="btn-primary">Reply by email</a>`)
	assert.NotContains(t, html, "ZgotmplZ")
}

func TestAdminLayoutDoesNotCacheHistory(t *testing.T) {
	funcs := server.TemplateFuncs()
	funcs["render"] = func(string, any) (template.HTML, error) { return "", nil }
	tmpl, err := template.New("base").Funcs(funcs).ParseFS(web.Templates, "layouts/base.html")
	require.NoError(t, err)

	html := render(t, tmpl, "layouts/base", map[string]any{})

	assert.Contains(t, html, `<body hx-boost="true" hx-history="false"`)
	head, _, found := strings.Cut(html, "</head>")
	require.True(t, found)
	assert.Contains(t, head, `/assets/vendor/htmx.min.js?v=`)
	assert.Contains(t, head, "defer")
	body := html[strings.Index(html, "<body"):]
	assert.NotContains(t, body, "/assets/vendor/htmx.min.js")
	assert.Contains(t, html, `localStorage.removeItem('htmx-history-cache')`)
	assert.Contains(t, html, `form[data-clear-history]`)
}

func TestSubmissionSearchSwapsOnlyTheResults(t *testing.T) {
	tmpl := parseTemplates(t, "admin/submissions/index.html", "admin/submissions/list.html")

	html := render(t, tmpl, "admin/submissions/index/content", map[string]any{
		"TotalCount":  int64(0),
		"SpamCount":   int64(0),
		"Submissions": []forms.Submission{},
		"Forms":       []forms.Form{},
		"Ranges":      []filterLink{},
		"SpamChoices": []filterLink{},
	})

	assert.Contains(t, html, `hx-target="#submission-results"`)
	assert.Contains(t, html, `hx-select="#submission-results"`)
	formEnd := strings.Index(html, "</form>")
	require.NotEqual(t, -1, formEnd)
	assert.Contains(t, html[:formEnd], `id="submission-search"`)
	assert.NotContains(t, html[:formEnd], `id="submission-results"`)
	assert.Contains(t, html[formEnd:], `id="submission-results"`)
}

func parseTemplates(t *testing.T, files ...string) *template.Template {
	t.Helper()
	tmpl, err := template.New("page").Funcs(server.TemplateFuncs()).ParseFS(web.Templates, files...)
	require.NoError(t, err)
	return tmpl
}

func render(t *testing.T, tmpl *template.Template, name string, data any) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&buf, name, data))
	return buf.String()
}
