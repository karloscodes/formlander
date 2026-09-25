package http

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
)

// A plain HTML form post is a browser navigation, so the visitor sees our
// response as a page. fetch() and the SDK get JSON as before.
func wantsHTML(ctx *cartridge.Context) bool {
	switch ctx.Get("Sec-Fetch-Mode") {
	case "navigate":
		return true
	case "":
		accept := ctx.Get(fiber.HeaderAccept)
		return strings.Contains(accept, "text/html") && !strings.Contains(accept, "application/json")
	default:
		return false
	}
}

// submitError answers a failed submission as a page or as JSON.
func submitError(ctx *cartridge.Context, status int, message string) error {
	if !wantsHTML(ctx) {
		return jsonError(ctx, status, message)
	}
	title, body := visitorMessage(status, message)
	return renderSubmitPage(ctx, status, submitPage{
		Title:   title,
		Body:    body,
		Details: message,
		BackURL: backURL(ctx),
		IsError: true,
	})
}

// submitSuccess sends the browser to the thank-you page when the form has
// no _success_url. The redirect (Post/Redirect/Get) keeps a page refresh
// from posting the form a second time.
func submitSuccess(ctx *cartridge.Context) error {
	return ctx.Redirect("/forms/sent", fiber.StatusSeeOther)
}

// SubmissionSent renders the thank-you page after a browser submission.
func SubmissionSent(ctx *cartridge.Context) error {
	return renderSubmitPage(ctx, fiber.StatusOK, submitPage{
		Title:   "Thanks, we got it.",
		Body:    "Your message is on its way. You can close this page or go back.",
		BackURL: backURL(ctx),
	})
}

// visitorMessage turns an API error into words for the person who filled
// in the form. The raw message stays below it for the form's owner.
func visitorMessage(status int, message string) (string, string) {
	switch {
	case status == fiber.StatusNotFound:
		return "This form doesn't exist.", "The form may have been removed. Please contact the site owner another way."
	case status == fiber.StatusUnauthorized:
		return "This form's link is not valid.", "The site owner needs to update the form. Please contact them another way."
	case status == fiber.StatusForbidden:
		return "This form can't be sent from this site.", "The site owner needs to allow this website in the form's settings."
	case status == fiber.StatusTooManyRequests:
		return "Too many tries.", "Please wait a minute and send the form again."
	case strings.Contains(message, "captcha"):
		return "Please complete the captcha.", "Go back, complete the check above the button, and send the form again."
	case status >= 500:
		return "Something went wrong on our side.", "Your message was not saved. Please go back and try again in a moment."
	default:
		return "Something in the form wasn't right.", "Please go back, check the fields, and send it again."
	}
}

// backURL is the fallback for the Go back button. Browsers often send only
// the site's origin as Referer on cross-site posts, so the button uses the
// browser history first.
func backURL(ctx *cartridge.Context) string {
	ref, err := url.Parse(ctx.Get(fiber.HeaderReferer))
	if err != nil || (ref.Scheme != "http" && ref.Scheme != "https") || ref.Host == "" {
		return ""
	}
	return ref.String()
}

type submitPage struct {
	Title   string
	Body    string
	Details string
	BackURL string
	IsError bool
}

func renderSubmitPage(ctx *cartridge.Context, status int, page submitPage) error {
	var buf bytes.Buffer
	if err := submitPageTemplate.Execute(&buf, page); err != nil {
		return jsonError(ctx, status, page.Details)
	}
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return ctx.Status(status).Send(buf.Bytes())
}

var submitPageTemplate = template.Must(template.New("submit").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{ .Title }}</title>
<style>
	*, *::before, *::after { box-sizing: border-box; }
	body {
		margin: 0;
		min-height: 100vh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 20px;
		padding: 24px 16px;
		background: #f5f5f4;
		color: #1c1917;
		font-family: Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif;
		line-height: 1.5;
	}
	.card {
		width: 100%;
		max-width: 440px;
		padding: 32px;
		background: #ffffff;
		border-radius: 16px;
		box-shadow: 0 1px 2px rgba(28, 25, 23, 0.04), 0 0 0 1px rgba(28, 25, 23, 0.08);
	}
	.icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		border-radius: 9999px;
		background: #ecfdf5;
		color: #059669;
	}
	.icon.error { background: #fef2f2; color: #dc2626; }
	h1 { margin: 20px 0 8px; font-size: 1.375rem; line-height: 1.3; font-weight: 600; letter-spacing: -0.02em; }
	p { margin: 0; color: #44403c; }
	.details {
		margin-top: 20px;
		padding: 10px 12px;
		border-radius: 8px;
		background: #f5f5f4;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.8rem;
		color: #57534e;
		word-break: break-word;
	}
	.button {
		display: inline-block;
		margin-top: 24px;
		padding: 10px 18px;
		border-radius: 8px;
		background: #1c1917;
		color: #ffffff;
		font-weight: 500;
		text-decoration: none;
	}
	.button:hover { opacity: 0.88; }
	.promo { font-size: 0.8rem; color: #78716c; }
	.promo a { color: #44403c; font-weight: 500; text-decoration: none; }
	.promo a:hover { text-decoration: underline; }
</style>
</head>
<body>
	<main class="card">
		{{ if .IsError }}
		<div class="icon error" aria-hidden="true">
			<svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM9 6a1 1 0 1 1 2 0v4a1 1 0 1 1-2 0V6zm1 8.25a1.25 1.25 0 1 0 0-2.5 1.25 1.25 0 0 0 0 2.5z" clip-rule="evenodd"/></svg>
		</div>
		{{ else }}
		<div class="icon" aria-hidden="true">
			<svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M16.7 5.3a1 1 0 0 1 0 1.4l-8 8a1 1 0 0 1-1.4 0l-4-4a1 1 0 1 1 1.4-1.4L8 12.58l7.3-7.3a1 1 0 0 1 1.4 0z" clip-rule="evenodd"/></svg>
		</div>
		{{ end }}
		<h1>{{ .Title }}</h1>
		<p>{{ .Body }}</p>
		{{ if .Details }}<div class="details">{{ .Details }}</div>{{ end }}
		<a class="button" href="{{ if .BackURL }}{{ .BackURL }}{{ else }}#{{ end }}" onclick="if (history.length > 1) { history.back(); return false; }">Go back</a>
	</main>
	<p class="promo">Form by <a href="https://formlander.com/?ref=form">Formlander</a>, the free form backend for static sites.</p>
</body>
</html>
`))
