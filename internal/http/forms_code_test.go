package http

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/karloscodes/cartridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/forms"
	"formlander/internal/server"
	"formlander/web"
)

func TestFormPagesRenderDeleteAndCaptchaHint(t *testing.T) {
	views := cartridge.NewHTMLViews(web.Templates, server.TemplateFuncs(), false)
	form := &forms.Form{ID: 3, Name: "Contact", PublicID: "abc", Token: "tok", Slug: "contact"}

	var show bytes.Buffer
	err := views.Render(&show, "layouts/base", map[string]any{
		"Title":           "Contact",
		"Form":            form,
		"Submissions":     []forms.Submission{},
		"SubmissionCount": int64(2),
		"DeleteConfirm":   deleteFormConfirm(2),
		"ActionURL":       "https://forms.example.com/forms/contact/submit?token=tok",
		"Token":           "tok",
		"FormCode":        "<form></form>",
		"CodeSplit":       true,
		"ContentView":     "admin/forms/show/content",
	})
	require.NoError(t, err)
	page := show.String()
	assert.Contains(t, page, "Delete this form and its 2 submissions? You cannot undo it.")
	assert.Contains(t, page, `onsubmit="return confirm('Delete this form and its 2 submissions? You cannot undo it.')"`)
	assert.Contains(t, page, `action="/admin/forms/3/delete"`)
	assert.Contains(t, page, "Turnstile stops robots.")
	assert.Contains(t, page, `href="/admin/settings/captcha"`)

	var editor bytes.Buffer
	err = views.Render(&editor, "layouts/base", map[string]any{
		"Title":       "New Form",
		"Input":       formInput{Name: "Contact"},
		"ContentView": "admin/forms/new/content",
	})
	require.NoError(t, err)
	assert.Contains(t, editor.String(), "Turnstile stops robots.")
	assert.Contains(t, editor.String(), `href="/admin/settings/captcha/new"`)
}

func TestBuildDefaultFormCodeIncludesHoneypot(t *testing.T) {
	form := &forms.Form{ID: 4, Name: "Contact", Slug: "contact", PublicID: "pub", Token: "tok"}
	field := defaultHoneypotField()
	// The starter field, plus a style. The starter stylesheet is what hides
	// the field there, and this snippet does not include that stylesheet.
	assert.Contains(t, field, strings.TrimSuffix(honeypotField, ">"))
	assert.Contains(t, field, `style="display:none"`)

	code := buildDefaultFormCode("https://forms.example.com/forms/contact/submit?token=tok", form, nil)

	start := strings.Index(code, "<form")
	fieldAt := strings.Index(code, field)
	end := strings.Index(code, "</form>")
	require.Greater(t, fieldAt, start)
	require.Less(t, fieldAt, end)

	withCaptcha := buildDefaultFormCode("https://forms.example.com/forms/contact/submit?token=tok", form, &captchaEmbed{
		WidgetMarkup: `    <div class="cf-turnstile" data-sitekey="site"></div>`,
		ScriptTag:    `<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>`,
	})
	assert.Contains(t, withCaptcha, field)
	assert.Contains(t, withCaptcha, "cf-turnstile")
	assert.Less(t, strings.Index(withCaptcha, field), strings.Index(withCaptcha, "</form>"))
}

func TestReadmeExampleIncludesHoneypot(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	assert.Contains(t, string(readme), defaultHoneypotField())
}

func TestDeleteFormConfirmNamesTheCount(t *testing.T) {
	assert.Equal(t, "Delete this form and its 0 submissions? You cannot undo it.", deleteFormConfirm(0))
	assert.Equal(t, "Delete this form and its 1 submission? You cannot undo it.", deleteFormConfirm(1))
	assert.Equal(t, "Delete this form and its 12 submissions? You cannot undo it.", deleteFormConfirm(12))

	page, err := os.ReadFile("../../web/templates/admin/forms/show.html")
	require.NoError(t, err)
	body := string(page)
	assert.Contains(t, body, "{{ .DeleteConfirm }}")
	assert.Contains(t, body, `/admin/forms/{{ .Form.ID }}/delete`)
	assert.Contains(t, body, `href="/admin/settings/captcha"`)
}
