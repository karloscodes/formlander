package http

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"formlander/internal/forms"
	"formlander/internal/server"
	"formlander/web"
)

// Reply by email renders a real mailto URL. html/template leaves this address
// alone: it is not replaced with #ZgotmplZ and the @ is not percent-encoded.
func TestSubmissionShowReplyHref(t *testing.T) {
	tmpl := parseTemplates(t, "admin/submissions/show.html", "admin/deliveries.html")

	address := "reply@example.com"
	submission := forms.Submission{
		ID:        7,
		DataJSON:  `{"name":"Ada","email":"` + address + `"}`,
		CreatedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
	}
	require.Equal(t, address, submission.ReplyAddress())

	page := render(t, tmpl, "admin/submissions/show/content", map[string]any{
		"Submission": submission,
		"ReplyTo":    submission.ReplyAddress(),
		"JSON":       "{}",
		"ReturnTo":   "/admin/submissions",
	})

	doc, err := html.Parse(strings.NewReader(page))
	require.NoError(t, err)
	var reply, copy *html.Node
	walkElements(doc, func(n *html.Node) {
		if n.Data == "a" && attr(n, "class") == "btn-primary" {
			reply = n
		}
		if n.Data == "button" && textContent(n) == "Copy address" {
			copy = n
		}
	})
	require.NotNil(t, reply)
	assert.Equal(t, "mailto:"+address, attr(reply, "href"))
	assert.Empty(t, attr(reply, "hx-boost"))
	require.NotNil(t, copy)
	assert.Equal(t, address, attr(copy, "data-address"))
	assert.Contains(t, attr(copy, "onclick"), "copyText")
}

func TestAdminLayoutDoesNotCacheHistory(t *testing.T) {
	funcs := server.TemplateFuncs()
	funcs["render"] = func(string, any) (template.HTML, error) { return "", nil }
	tmpl, err := template.New("base").Funcs(funcs).ParseFS(web.Templates, "layouts/base.html")
	require.NoError(t, err)

	page := render(t, tmpl, "layouts/base", map[string]any{})

	doc, err := html.Parse(strings.NewReader(page))
	require.NoError(t, err)
	body := findElement(doc, func(n *html.Node) bool { return n.Data == "body" })
	require.NotNil(t, body)
	assert.Equal(t, "true", attr(body, "hx-boost"))
	assert.Equal(t, "false", attr(body, "hx-history"))

	var htmxScript, inline *html.Node
	walkElements(doc, func(n *html.Node) {
		if n.Data != "script" {
			return
		}
		if strings.Contains(attr(n, "src"), "htmx.min.js") {
			htmxScript = n
		}
		if n.FirstChild != nil && strings.Contains(n.FirstChild.Data, "htmx-history-cache") {
			inline = n
		}
	})
	require.NotNil(t, htmxScript)
	assert.True(t, hasAttr(htmxScript, "defer"))
	assert.Equal(t, "head", htmxScript.Parent.Data)
	require.NotNil(t, inline)
	assert.NotContains(t, inline.FirstChild.Data, "htmx.")
	assert.NotContains(t, inline.FirstChild.Data, "data-clear-history")
}

func TestSubmissionSearchSwapsOnlyTheResults(t *testing.T) {
	tmpl := parseTemplates(t, "admin/submissions/index.html", "admin/submissions/list.html")

	page := render(t, tmpl, "admin/submissions/index/content", map[string]any{
		"TotalCount":  int64(0),
		"SpamCount":   int64(0),
		"Submissions": []forms.Submission{},
		"Forms":       []forms.Form{},
		"Search":      "ada",
		"Ranges":      []filterLink{{Label: "7 days", URL: submissionsURL("", "7d", "", "ada", 1)}},
		"SpamChoices": []filterLink{{Label: "No spam", URL: submissionsURL("", "", "no", "ada", 1)}},
	})

	doc, err := html.Parse(strings.NewReader(page))
	require.NoError(t, err)
	form := findElement(doc, func(n *html.Node) bool {
		return n.Data == "form" && hasAttr(n, "data-live-search")
	})
	require.NotNil(t, form)
	assert.Equal(t, "#submission-results", attr(form, "hx-target"))
	assert.Equal(t, "#submission-results", attr(form, "hx-select"))
	assert.Equal(t, "outerHTML", attr(form, "hx-swap"))
	assert.Equal(t, "#submission-filters", attr(form, "hx-select-oob"))
	assert.Empty(t, attr(form, "hx-get"))
	assert.Empty(t, attr(form, "hx-push-url"))

	search := findElement(form, func(n *html.Node) bool { return attr(n, "id") == "submission-search" })
	results := findElement(doc, func(n *html.Node) bool { return attr(n, "id") == "submission-results" })
	filters := findElement(form, func(n *html.Node) bool { return attr(n, "id") == "submission-filters" })
	require.NotNil(t, search)
	require.NotNil(t, results)
	require.NotNil(t, filters)
	assert.False(t, containsElement(results, search))
	week := findElement(filters, func(n *html.Node) bool { return n.Data == "a" && textContent(n) == "7 days" })
	require.NotNil(t, week)
	assert.Equal(t, "/admin/submissions?q=ada&range=7d", attr(week, "href"))
}

func TestSubmissionsFilterLinksKeepTheSearch(t *testing.T) {
	assert.Equal(t, "/admin/submissions?q=ada&range=7d", submissionsURL("", "7d", "", "ada", 1))
	assert.Equal(t, "/admin/submissions?q=ada&spam=no", submissionsURL("", "", "no", "ada", 1))
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

func walkElements(n *html.Node, visit func(*html.Node)) {
	if n.Type == html.ElementNode {
		visit(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkElements(c, visit)
	}
}

func findElement(n *html.Node, match func(*html.Node) bool) *html.Node {
	var found *html.Node
	walkElements(n, func(el *html.Node) {
		if found == nil && match(el) {
			found = el
		}
	})
	return found
}

func containsElement(root, target *html.Node) bool {
	return findElement(root, func(n *html.Node) bool { return n == target }) != nil
}

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func textContent(n *html.Node) string {
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(buf.String())
}
