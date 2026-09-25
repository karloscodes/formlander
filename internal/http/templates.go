package http

import (
	"formlander/internal/forms"
	"strings"
)

// FormTemplate represents a pre-configured form template.
type FormTemplate struct {
	ID              string
	Name            string
	Description     string
	Slug            string
	Icon            string
	Color           string
	ComingSoon      bool
	WIP             bool
	HTML            string
	WebhookDelivery forms.WebhookDelivery
	EmailDelivery   forms.EmailDelivery
}

// GetFormTemplates returns all available form templates.
func GetFormTemplates() []FormTemplate {
	return []FormTemplate{
		{
			ID:          "contact",
			Name:        "Contact Form",
			Description: "Name, email, and a message.",
			Slug:        "contact",
			Icon:        "💬",
			Color:       "blue",
			HTML:        contactTemplateHTML,
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:          "feedback",
			Name:        "Feedback Form",
			Description: "A quick rating and a comment.",
			Slug:        "feedback",
			Icon:        "💡",
			Color:       "purple",
			HTML:        feedbackTemplateHTML,
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:          "bug-report",
			Name:        "Bug Report",
			Description: "Severity, steps to reproduce, and what went wrong.",
			Slug:        "bug-report",
			Icon:        "🐛",
			Color:       "red",
			HTML:        bugTemplateHTML,
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:          "newsletter",
			Name:        "Newsletter Signup",
			Description: "An email address and consent to send.",
			Slug:        "newsletter",
			Icon:        "📧",
			Color:       "green",
			HTML:        newsletterTemplateHTML,
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:          "waitlist",
			Name:        "Waitlist",
			Description: "An email, plus a few optional details.",
			Slug:        "waitlist",
			Icon:        "⏳",
			Color:       "yellow",
			HTML:        waitlistTemplateHTML,
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:              "blank",
			Name:            "Blank Form",
			Description:     "Two fields. Build the rest yourself.",
			Slug:            "",
			Icon:            "📝",
			Color:           "gray",
			HTML:            blankTemplateHTML,
			EmailDelivery:   forms.EmailDelivery{},
			WebhookDelivery: forms.WebhookDelivery{},
		},
	}
}

// GetTemplateByID returns a specific template by ID.
func GetTemplateByID(id string) *FormTemplate {
	templates := GetFormTemplates()
	for _, t := range templates {
		if t.ID == id {
			template := t // copy to avoid referencing loop variable
			return &template
		}
	}
	return nil
}

// RenderHTML returns the template HTML with the form action placeholder replaced.
func (t *FormTemplate) RenderHTML(action string) string {
	if t == nil || strings.TrimSpace(t.HTML) == "" {
		return ""
	}

	if strings.TrimSpace(action) == "" {
		action = "/forms/your-form/submit?token=YOUR_FORM_TOKEN"
	}

	return strings.ReplaceAll(t.HTML, "{{FORM_ACTION}}", action)
}

// sharedTemplateStyles is scoped to .formlander-shell so it cannot restyle
// the page that embeds the form. --fl-accent colors the button and the
// focus ring; change that one value to match your brand.
const sharedTemplateStyles = `<style>
	.formlander-shell {
		--fl-accent: #1c1917;
		box-sizing: border-box;
		max-width: 480px;
		margin: 24px auto;
		padding: 32px;
		background: #ffffff;
		border-radius: 16px;
		box-shadow: 0 1px 2px rgba(28, 25, 23, 0.04), 0 0 0 1px rgba(28, 25, 23, 0.08);
		font-family: Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif;
		line-height: 1.5;
		color: #1c1917;
		color-scheme: light;
	}

	.formlander-shell *,
	.formlander-shell *::before,
	.formlander-shell *::after {
		box-sizing: inherit;
	}

	.formlander-shell .formlander-eyebrow {
		margin: 0;
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: #78716c;
	}

	.formlander-shell h2 {
		margin: 6px 0 6px;
		font-size: 1.5rem;
		line-height: 1.25;
		font-weight: 600;
		letter-spacing: -0.02em;
	}

	.formlander-shell p {
		margin: 0;
		font-size: 0.95rem;
		color: #44403c;
	}

	.formlander-stack {
		display: grid;
		gap: 16px;
		margin-top: 24px;
	}

	.formlander-row {
		display: grid;
		gap: 16px;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	}

	.formlander-field {
		display: block;
		min-width: 0;
	}

	.formlander-label {
		display: block;
		margin-bottom: 6px;
		font-size: 0.875rem;
		font-weight: 500;
		color: #1c1917;
	}

	.formlander-optional {
		font-weight: 400;
		color: #78716c;
	}

	.formlander-field input,
	.formlander-field select,
	.formlander-field textarea {
		display: block;
		width: 100%;
		margin: 0;
		padding: 10px 12px;
		font: inherit;
		font-size: 1rem;
		color: #1c1917;
		background: #ffffff;
		border: 1px solid #d6d3d1;
		border-radius: 8px;
		transition: border-color 0.15s ease, box-shadow 0.15s ease;
	}

	.formlander-field select {
		appearance: none;
		padding-right: 36px;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 20 20' fill='%2378716c'%3E%3Cpath d='M5.3 7.3a1 1 0 0 1 1.4 0L10 10.6l3.3-3.3a1 1 0 1 1 1.4 1.4l-4 4a1 1 0 0 1-1.4 0l-4-4a1 1 0 0 1 0-1.4z'/%3E%3C/svg%3E");
		background-repeat: no-repeat;
		background-position: right 12px center;
		background-size: 16px;
	}

	.formlander-field textarea {
		min-height: 112px;
		resize: vertical;
	}

	.formlander-field ::placeholder {
		color: #a8a29e;
	}

	.formlander-field input:focus,
	.formlander-field select:focus,
	.formlander-field textarea:focus {
		outline: none;
		border-color: var(--fl-accent);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--fl-accent) 20%, transparent);
	}

	.formlander-helper {
		margin-top: 6px;
		font-size: 0.8rem;
		color: #78716c;
	}

	.formlander-checkbox {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		font-size: 0.875rem;
		color: #44403c;
	}

	.formlander-checkbox input {
		flex: none;
		width: 16px;
		height: 16px;
		margin: 3px 0 0;
		accent-color: var(--fl-accent);
	}

	.formlander-button {
		width: 100%;
		padding: 12px 20px;
		font: inherit;
		font-weight: 500;
		color: #ffffff;
		background: var(--fl-accent);
		border: 0;
		border-radius: 8px;
		cursor: pointer;
		transition: opacity 0.15s ease;
	}

	.formlander-button:hover {
		opacity: 0.88;
	}

	.formlander-button:focus-visible {
		outline: 2px solid var(--fl-accent);
		outline-offset: 2px;
	}

	.formlander-hp {
		display: none !important;
	}

	.formlander-shell .formlander-powered {
		margin-top: 16px;
		font-size: 0.75rem;
		text-align: center;
		color: #a8a29e;
	}

	.formlander-powered a {
		color: #78716c;
		text-decoration: none;
	}

	.formlander-powered a:hover {
		text-decoration: underline;
	}

	@media (max-width: 520px) {
		.formlander-shell {
			margin: 16px;
			padding: 24px;
		}
	}
</style>
`

// honeypotField is left empty by people and filled by bots. Formlander
// flags any submission where it has a value.
const honeypotField = `<input type="text" name="__fl_hp" class="formlander-hp" tabindex="-1" autocomplete="off" aria-hidden="true">`

const contactTemplateHTML = sharedTemplateStyles + `
<div class="formlander-shell">
	<p class="formlander-eyebrow">Contact</p>
	<h2>Get in touch</h2>
	<p>Send us a message and we'll reply by email.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<!-- Add, rename, or remove fields freely. Formlander saves whatever this form sends. -->
		<div class="formlander-row">
			<label class="formlander-field">
				<span class="formlander-label">Name</span>
				<input type="text" name="name" autocomplete="name" required>
			</label>
			<label class="formlander-field">
				<span class="formlander-label">Email</span>
				<input type="email" name="email" autocomplete="email" placeholder="you@example.com" required>
			</label>
		</div>

		<label class="formlander-field">
			<span class="formlander-label">Topic <span class="formlander-optional">(optional)</span></span>
			<select name="topic">
				<option value="">Choose a topic</option>
				<option>Question</option>
				<option>Billing</option>
				<option>Partnership</option>
				<option>Something else</option>
			</select>
		</label>

		<label class="formlander-field">
			<span class="formlander-label">Message</span>
			<textarea name="message" required></textarea>
		</label>

		` + honeypotField + `
		<button type="submit" class="formlander-button">Send message</button>
	</form>
	<p class="formlander-powered">Form by <a href="https://formlander.com/?ref=form">Formlander</a></p>
</div>
`

const newsletterTemplateHTML = sharedTemplateStyles + `
<div class="formlander-shell">
	<p class="formlander-eyebrow">Newsletter</p>
	<h2>Get the newsletter</h2>
	<p>Product news and useful notes. Unsubscribe any time.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<!-- Add, rename, or remove fields freely. Formlander saves whatever this form sends. -->
		<label class="formlander-field">
			<span class="formlander-label">Email</span>
			<input type="email" name="email" autocomplete="email" placeholder="you@example.com" required>
		</label>

		<label class="formlander-field">
			<span class="formlander-label">First name <span class="formlander-optional">(optional)</span></span>
			<input type="text" name="first_name" autocomplete="given-name">
		</label>

		<label class="formlander-checkbox">
			<input type="checkbox" name="consent" value="yes" required>
			<span>Send me the newsletter. I can unsubscribe at any time.</span>
		</label>

		` + honeypotField + `
		<button type="submit" class="formlander-button">Subscribe</button>
	</form>
	<p class="formlander-powered">Form by <a href="https://formlander.com/?ref=form">Formlander</a></p>
</div>
`

const waitlistTemplateHTML = sharedTemplateStyles + `
<div class="formlander-shell">
	<p class="formlander-eyebrow">Waitlist</p>
	<h2>Join the waitlist</h2>
	<p>Leave your email and we'll let you know when it's ready.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<!-- Add, rename, or remove fields freely. Formlander saves whatever this form sends. -->
		<label class="formlander-field">
			<span class="formlander-label">Email</span>
			<input type="email" name="email" autocomplete="email" placeholder="you@example.com" required>
		</label>

		<div class="formlander-row">
			<label class="formlander-field">
				<span class="formlander-label">Name <span class="formlander-optional">(optional)</span></span>
				<input type="text" name="name" autocomplete="name">
			</label>
			<label class="formlander-field">
				<span class="formlander-label">Company <span class="formlander-optional">(optional)</span></span>
				<input type="text" name="company" autocomplete="organization">
			</label>
		</div>

		<label class="formlander-field">
			<span class="formlander-label">What would you use it for? <span class="formlander-optional">(optional)</span></span>
			<textarea name="use_case"></textarea>
		</label>

		` + honeypotField + `
		<button type="submit" class="formlander-button">Join the waitlist</button>
	</form>
	<p class="formlander-powered">Form by <a href="https://formlander.com/?ref=form">Formlander</a></p>
</div>
`

const feedbackTemplateHTML = sharedTemplateStyles + `
<div class="formlander-shell">
	<p class="formlander-eyebrow">Feedback</p>
	<h2>Tell us what you think</h2>
	<p>What works, what doesn't, what's missing. We read every message.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<!-- Add, rename, or remove fields freely. Formlander saves whatever this form sends. -->
		<label class="formlander-field">
			<span class="formlander-label">How is it going?</span>
			<select name="satisfaction" required>
				<option value="">Choose one</option>
				<option>Great</option>
				<option>Good</option>
				<option>Okay</option>
				<option>Not good</option>
			</select>
		</label>

		<label class="formlander-field">
			<span class="formlander-label">Your feedback</span>
			<textarea name="comments" required></textarea>
		</label>

		<div class="formlander-row">
			<label class="formlander-field">
				<span class="formlander-label">Name <span class="formlander-optional">(optional)</span></span>
				<input type="text" name="name" autocomplete="name">
			</label>
			<label class="formlander-field">
				<span class="formlander-label">Email <span class="formlander-optional">(optional)</span></span>
				<input type="email" name="email" autocomplete="email" placeholder="you@example.com">
			</label>
		</div>

		` + honeypotField + `
		<button type="submit" class="formlander-button">Send feedback</button>
	</form>
	<p class="formlander-powered">Form by <a href="https://formlander.com/?ref=form">Formlander</a></p>
</div>
`

const bugTemplateHTML = sharedTemplateStyles + `
<div class="formlander-shell">
	<p class="formlander-eyebrow">Bug report</p>
	<h2>Report a bug</h2>
	<p>Tell us what happened and how to make it happen again.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<!-- Add, rename, or remove fields freely. Formlander saves whatever this form sends. -->
		<div class="formlander-row">
			<label class="formlander-field">
				<span class="formlander-label">Name</span>
				<input type="text" name="reporter" autocomplete="name" required>
			</label>
			<label class="formlander-field">
				<span class="formlander-label">Email</span>
				<input type="email" name="email" autocomplete="email" placeholder="you@example.com" required>
			</label>
		</div>

		<div class="formlander-row">
			<label class="formlander-field">
				<span class="formlander-label">Severity</span>
				<select name="severity" required>
					<option value="">Choose one</option>
					<option>Low</option>
					<option>Medium</option>
					<option>High</option>
					<option>Critical</option>
				</select>
			</label>
			<label class="formlander-field">
				<span class="formlander-label">Where <span class="formlander-optional">(optional)</span></span>
				<input type="text" name="area" placeholder="Page or feature">
			</label>
		</div>

		<label class="formlander-field">
			<span class="formlander-label">Steps to reproduce</span>
			<textarea name="steps" placeholder="1. Go to&#10;2. Click&#10;3. See the error" required></textarea>
		</label>

		<label class="formlander-field">
			<span class="formlander-label">Expected vs. actual <span class="formlander-optional">(optional)</span></span>
			<textarea name="expected" placeholder="I expected X, but saw Y"></textarea>
		</label>

		` + honeypotField + `
		<button type="submit" class="formlander-button">Send bug report</button>
	</form>
	<p class="formlander-powered">Form by <a href="https://formlander.com/?ref=form">Formlander</a></p>
</div>
`

const blankTemplateHTML = sharedTemplateStyles + `
<div class="formlander-shell">
	<p class="formlander-eyebrow">Form</p>
	<h2>Your form title</h2>
	<p>One line about what this form is for.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<!-- Add, rename, or remove fields freely. Formlander saves whatever this form sends. -->
		<label class="formlander-field">
			<span class="formlander-label">Field label</span>
			<input type="text" name="field_one">
		</label>

		<label class="formlander-field">
			<span class="formlander-label">Message</span>
			<textarea name="field_two"></textarea>
		</label>

		` + honeypotField + `
		<button type="submit" class="formlander-button">Submit</button>
	</form>
	<p class="formlander-powered">Form by <a href="https://formlander.com/?ref=form">Formlander</a></p>
</div>
`
