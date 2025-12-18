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
			Description: "Simple contact form with name, email, and message fields",
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
			Description: "Collect user feedback and feature requests",
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
			Description: "Help users report bugs and technical issues",
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
			Description: "Collect email addresses for your newsletter",
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
			Description: "Build a waitlist for your product launch",
			Slug:        "waitlist",
			Icon:        "⏳",
			Color:       "yellow",
			HTML:        waitlistTemplateHTML,
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:          "ai-powered",
			Name:        "AI-Powered Custom Form",
			Description: "Let AI build a custom form based on your requirements",
			Slug:        "ai-powered",
			Icon:        "🤖",
			Color:       "indigo",
			ComingSoon:  false,
			WIP:         false,
			HTML:        "", // Pro feature - handled by redirect
			EmailDelivery: forms.EmailDelivery{
				Enabled: true,
			},
		},
		{
			ID:              "blank",
			Name:            "Blank Form",
			Description:     "Start from scratch with an empty form",
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

const sharedTemplateStyles = `
<style>
	.formlander-shell {
		max-width: 520px;
		margin: 24px auto;
		background: #ffffff;
		border-radius: 20px;
		padding: 32px;
		border: 1px solid #e2e8f0;
		box-shadow: 0 25px 45px rgba(15, 23, 42, 0.08);
		font-family: 'Inter', system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
		color: #0f172a;
	}

	.formlander-eyebrow {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 4px 12px;
		border-radius: 9999px;
		font-size: 12px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		background: rgba(59, 130, 246, 0.12);
		color: #2563eb;
	}

	.formlander-shell h2 {
		font-size: 1.6rem;
		margin: 0.85rem 0 0.4rem;
	}

	.formlander-shell p {
		margin: 0;
		color: #64748b;
		font-size: 0.95rem;
	}

	.formlander-stack {
		display: flex;
		flex-direction: column;
		gap: 18px;
		margin-top: 1.5rem;
	}

	.formlander-row {
		display: flex;
		flex-wrap: wrap;
		gap: 16px;
	}

	.formlander-field {
		flex: 1;
		min-width: 160px;
	}

	.formlander-field span {
		display: block;
		font-size: 0.85rem;
		font-weight: 600;
		color: #475569;
		margin-bottom: 6px;
	}

	.formlander-field input,
	.formlander-field select,
	.formlander-field textarea {
		width: 100%;
		border: 1px solid #d0d7e3;
		border-radius: 14px;
		padding: 12px 14px;
		font-size: 0.95rem;
		transition: border 0.2s ease, box-shadow 0.2s ease;
		background: #f8fafc;
	}

	.formlander-field textarea {
		min-height: 120px;
		resize: vertical;
	}

	.formlander-field input:focus,
	.formlander-field textarea:focus,
	.formlander-field select:focus {
		outline: none;
		border-color: #2563eb;
		box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15);
		background: #ffffff;
	}

	.formlander-helper {
		font-size: 0.8rem;
		color: #94a3b8;
		margin-top: 4px;
	}

	.formlander-button {
		width: 100%;
		border: none;
		border-radius: 16px;
		padding: 14px 18px;
		font-size: 1rem;
		font-weight: 600;
		color: #ffffff;
		background: linear-gradient(135deg, #2563eb, #4338ca);
		cursor: pointer;
		transition: transform 0.2s ease, box-shadow 0.2s ease;
	}

	.formlander-button:hover {
		transform: translateY(-1px);
		box-shadow: 0 15px 30px rgba(37, 99, 235, 0.25);
	}

	.formlander-checkbox {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		font-size: 0.9rem;
		color: #475569;
	}

	.formlander-checkbox input {
		width: 18px;
		height: 18px;
		margin-top: 3px;
	}
</style>
`

const contactTemplateHTML = `
<div class="formlander-shell">
	<div class="formlander-eyebrow">Contact</div>
	<h2>Contact our team</h2>
	<p>Tell us how we can help and someone will reply within one business day.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<div class="formlander-row">
			<label class="formlander-field">
				<span>Full name</span>
				<input type="text" name="name" placeholder="Alex Rivers" required>
			</label>
			<label class="formlander-field">
				<span>Company</span>
				<input type="text" name="company" placeholder="Acme Inc.">
			</label>
		</div>

		<div class="formlander-row">
			<label class="formlander-field">
				<span>Email address</span>
				<input type="email" name="email" placeholder="you@example.com" required>
			</label>
			<label class="formlander-field">
				<span>Topic</span>
				<select name="topic" required>
					<option value="">Choose a topic</option>
					<option>Product question</option>
					<option>Billing</option>
					<option>Partnership</option>
					<option>Something else</option>
				</select>
			</label>
		</div>

		<label class="formlander-field">
			<span>How can we help?</span>
			<textarea name="message" placeholder="Share details about your request" required></textarea>
		</label>

		<label class="formlander-checkbox">
			<input type="checkbox" name="consent" required>
			<span>I agree to be contacted about this request.</span>
		</label>

		<button type="submit" class="formlander-button">Send message</button>
	</form>
</div>
` + sharedTemplateStyles

const newsletterTemplateHTML = `
<div class="formlander-shell">
	<div class="formlander-eyebrow" style="background: rgba(16, 185, 129, 0.18); color: #059669;">Newsletter</div>
	<h2>Join the newsletter</h2>
	<p>Receive product updates, launch notes, and best practices twice a month.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<label class="formlander-field">
			<span>Email address</span>
			<input type="email" name="email" placeholder="you@company.com" required>
		</label>

		<label class="formlander-field">
			<span>First name</span>
			<input type="text" name="first_name" placeholder="Jamie" required>
		</label>

		<label class="formlander-field">
			<span>What would you like to hear about?</span>
			<select name="interest">
				<option>Product updates</option>
				<option>Growth stories</option>
				<option>Weekly tips</option>
			</select>
		</label>

		<label class="formlander-checkbox">
			<input type="checkbox" name="consent" required>
			<span>I agree to receive occasional product emails.</span>
		</label>

		<button type="submit" class="formlander-button" style="background: linear-gradient(135deg, #059669, #047857);">Subscribe</button>
	</form>
</div>
` + sharedTemplateStyles

const waitlistTemplateHTML = `
<div class="formlander-shell">
	<div class="formlander-eyebrow" style="background: rgba(245, 158, 11, 0.16); color: #d97706;">Waitlist</div>
	<h2>Join the early access list</h2>
	<p>We’re releasing limited invites. Tell us a bit about your team and we’ll keep you posted.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<div class="formlander-row">
			<label class="formlander-field">
				<span>Full name</span>
				<input type="text" name="name" placeholder="Morgan Lee" required>
			</label>
			<label class="formlander-field">
				<span>Company</span>
				<input type="text" name="company" placeholder="Northwind">
			</label>
		</div>

		<div class="formlander-row">
			<label class="formlander-field">
				<span>Work email</span>
				<input type="email" name="email" placeholder="you@company.com" required>
			</label>
			<label class="formlander-field">
				<span>Team size</span>
				<select name="team_size">
					<option value="">Select</option>
					<option>1-5 people</option>
					<option>6-25 people</option>
					<option>26-100 people</option>
					<option>100+ people</option>
				</select>
			</label>
		</div>

		<label class="formlander-field">
			<span>What will you use us for?</span>
			<textarea name="use_case" placeholder="Share how your team would use the product" required></textarea>
		</label>

		<button type="submit" class="formlander-button" style="background: linear-gradient(135deg, #f59e0b, #d97706);">Request invite</button>
	</form>
</div>
` + sharedTemplateStyles

const feedbackTemplateHTML = `
<div class="formlander-shell">
	<div class="formlander-eyebrow" style="background: rgba(147, 51, 234, 0.15); color: #9333ea;">Feedback</div>
	<h2>Share your feedback</h2>
	<p>Help us build the roadmap. Tell us what’s working and what could be better.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<div class="formlander-row">
			<label class="formlander-field">
				<span>Name</span>
				<input type="text" name="name" placeholder="Taylor" required>
			</label>
			<label class="formlander-field">
				<span>Email</span>
				<input type="email" name="email" placeholder="you@example.com">
			</label>
		</div>

		<label class="formlander-field">
			<span>How satisfied are you?</span>
			<select name="satisfaction" required>
				<option value="">Choose a score</option>
				<option>Very satisfied</option>
				<option>Satisfied</option>
				<option>Neutral</option>
				<option>Unsatisfied</option>
			</select>
		</label>

		<label class="formlander-field">
			<span>Feature or area</span>
			<input type="text" name="feature" placeholder="Dashboard, Automations, ...">
		</label>

		<label class="formlander-field">
			<span>Comments</span>
			<textarea name="comments" placeholder="What should we improve?" required></textarea>
		</label>

		<button type="submit" class="formlander-button" style="background: linear-gradient(135deg, #a855f7, #7c3aed);">Send feedback</button>
	</form>
</div>
` + sharedTemplateStyles

const bugTemplateHTML = `
<div class="formlander-shell">
	<div class="formlander-eyebrow" style="background: rgba(248, 113, 113, 0.18); color: #dc2626;">Bug report</div>
	<h2>Report an issue</h2>
	<p>Found something off? Share the details and we’ll investigate within a few hours.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<div class="formlander-row">
			<label class="formlander-field">
				<span>Name</span>
				<input type="text" name="reporter" placeholder="Jordan" required>
			</label>
			<label class="formlander-field">
				<span>Email</span>
				<input type="email" name="email" placeholder="you@example.com" required>
			</label>
		</div>

		<div class="formlander-row">
			<label class="formlander-field">
				<span>Severity</span>
				<select name="severity" required>
					<option value="">Select severity</option>
					<option>Low</option>
					<option>Medium</option>
					<option>High</option>
					<option>Critical</option>
				</select>
			</label>
			<label class="formlander-field">
				<span>Area of the product</span>
				<input type="text" name="area" placeholder="Forms dashboard">
			</label>
		</div>

		<label class="formlander-field">
			<span>Steps to reproduce</span>
			<textarea name="steps" placeholder="1. Go to..., 2. Click..." required></textarea>
			<div class="formlander-helper">Include as much detail as possible.</div>
		</label>

		<label class="formlander-field">
			<span>Expected vs. actual behavior</span>
			<textarea name="expected" placeholder="Expected X but saw Y"></textarea>
		</label>

		<button type="submit" class="formlander-button" style="background: linear-gradient(135deg, #ef4444, #b91c1c);">Submit bug</button>
	</form>
</div>
` + sharedTemplateStyles

const blankTemplateHTML = `
<div class="formlander-shell">
	<div class="formlander-eyebrow">Simple form</div>
	<h2>Let’s collect data</h2>
	<p>Use this lightweight template as a starting point.</p>

	<form action="{{FORM_ACTION}}" method="POST" class="formlander-stack">
		<label class="formlander-field">
			<span>Field label</span>
			<input type="text" name="field_one" placeholder="Text input">
		</label>

		<label class="formlander-field">
			<span>Message</span>
			<textarea name="field_two" placeholder="Textarea input"></textarea>
		</label>

		<button type="submit" class="formlander-button">Submit</button>
	</form>
</div>
` + sharedTemplateStyles
