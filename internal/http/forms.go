package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	htmlnode "golang.org/x/net/html"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/cartridge"
	"formlander/internal/pkg/dbtxn"

	"go.uber.org/zap"
)

// AdminFormsIndex renders the list of forms.
func AdminFormsIndex(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	formsList, err := forms.List(db)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	// Load relations for display
	for i := range formsList {
		db.Preload("EmailDelivery").Preload("WebhookDelivery").First(&formsList[i], formsList[i].ID)
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Forms",
		"Forms":       formsList,
		"CreateRoute": "/admin/forms/new",
		"ContentView": "admin/forms/index/content",
	}, "")
}

// AdminFormsNew renders the new form view or template selector.
func AdminFormsNew(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	// Check if a template is selected
	templateID := ctx.Query("template")
	if templateID == "" {
		// Show template selector
		templates := GetFormTemplates()
		return ctx.Render("layouts/base", fiber.Map{
			"Title":       "Choose a Template",
			"Templates":   templates,
			"ContentView": "admin/forms/templates/content",
		}, "")
	}

	// Load template
	template := GetTemplateByID(templateID)
	if template == nil {
		return ctx.Redirect("/admin/forms/new")
	}

	// Load profiles for dropdowns
	mailerProfiles, _ := integrations.ListMailerProfiles(db)
	captchaProfiles, _ := integrations.ListCaptchaProfiles(db)

	// Pre-fill form with template data
	emailDelivery := template.EmailDelivery
	webhookDelivery := template.WebhookDelivery

	// Extract email recipient from overrides for display
	emailRecipient := ""
	if emailDelivery.OverridesJSON != "" {
		var overrides map[string]interface{}
		if err := json.Unmarshal([]byte(emailDelivery.OverridesJSON), &overrides); err == nil {
			if to, ok := overrides["to"].(string); ok {
				emailRecipient = to
			}
		}
	}

	previewHTML := template.RenderHTML(exampleFormAction(template.Slug))

	return ctx.Render("layouts/base", fiber.Map{
		"Title":                    "New Form",
		"DefaultSlug":              template.Slug,
		"FormName":                 template.Name,
		"EmailDelivery":            &emailDelivery,
		"WebhookDelivery":          &webhookDelivery,
		"EmailRecipient":           emailRecipient,
		"EmailEnabled":             emailDelivery.Enabled,
		"WebhookEnabled":           webhookDelivery.Enabled,
		"Template":                 template,
		"TemplateID":               template.ID,
		"PreviewHTML":              previewHTML,
		"MailerProfiles":           mailerProfiles,
		"CaptchaProfiles":          captchaProfiles,
		"SelectedMailerProfileID":  uint(0),
		"SelectedCaptchaProfileID": uint(0),
		"ContentView":              "admin/forms/new/content",
	}, "")
}

// AdminFormsCreate persists a new form configuration.
func AdminFormsCreate(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	cfg := ctx.Config
	templateID := strings.TrimSpace(ctx.FormValue("template_id"))
	selectedTemplate := GetTemplateByID(templateID)

	// Parse mailer profile ID if provided
	var mailerProfileID *uint
	if mailerIDStr := ctx.FormValue("mailer_profile_id"); mailerIDStr != "" {
		if id, err := strconv.ParseUint(mailerIDStr, 10, 32); err == nil {
			uid := uint(id)
			mailerProfileID = &uid
		}
	}

	// Parse captcha profile ID if provided
	var captchaProfileID *uint
	if captchaIDStr := ctx.FormValue("captcha_profile_id"); captchaIDStr != "" {
		if id, err := strconv.ParseUint(captchaIDStr, 10, 32); err == nil {
			uid := uint(id)
			captchaProfileID = &uid
		}
	}

	// Use forms context for business logic
	params := forms.CreateParams{
		Name:               ctx.FormValue("name"),
		Slug:               ctx.FormValue("slug"),
		AllowedOrigins:     ctx.FormValue("allowed_origins"),
		GeneratedHTML:      ctx.FormValue("generated_html"),
		MailerProfileID:    mailerProfileID,
		CaptchaProfileID:   captchaProfileID,
		EmailRecipient:     ctx.FormValue("email_recipient"),
		EmailEnabled:       ctx.FormValue("email_enabled") == "on",
		WebhookEnabled:     ctx.FormValue("webhook_enabled") == "on",
		WebhookURL:         ctx.FormValue("webhook_url"),
		WebhookSecret:      ctx.FormValue("webhook_secret"),
		WebhookHeadersJSON: ctx.FormValue("webhook_headers"),
		TemplateID:         templateID,
	}

	form, err := forms.Create(ctx.Logger, db, params)
	if err != nil {
		// Handle validation errors
		if validationErr, ok := err.(*forms.ValidationError); ok {
			return renderFormError(ctx, cfg, validationErr.Message, nil, nil, nil, false, selectedTemplate)
		}
		ctx.Logger.Error("failed to create form", zap.Error(err))
		return fiber.ErrInternalServerError
	}

	// Update generated HTML if template was selected
	if selectedTemplate != nil {
		if html := selectedTemplate.RenderHTML(liveFormAction(form.Slug, form.Token)); strings.TrimSpace(html) != "" {
			form.GeneratedHTML = html
			if err := dbtxn.WithRetry(ctx.Logger, db, func(tx *gorm.DB) error {
				return tx.Model(form).Update("generated_html", html).Error
			}); err != nil {
				ctx.Logger.Error("failed to update generated HTML", zap.Error(err))
			}
		}
	}

	return ctx.Redirect(fmt.Sprintf("/admin/forms/%d", form.ID))
}

// AdminFormShow displays a form summary and recent submissions.
func AdminFormShow(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	logger := ctx.Logger

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	form, err := forms.GetByID(db, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	// Ensure delivery records exist
	if err := forms.EnsureDeliveryRecords(logger, db, form); err != nil {
		// Log but don't fail - continue showing the form
		logger.Error("failed to ensure delivery records", zap.Error(err))
	}

	submissions, err := forms.GetSubmissions(db, form.ID, 25)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	webhookEvents, err := forms.GetWebhookEvents(db, form.ID, 20)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	emailEvents, err := forms.GetEmailEvents(db, form.ID, 20)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	// Extract email recipient from overrides for display
	emailRecipient := ""
	if form.EmailDelivery != nil && form.EmailDelivery.OverridesJSON != "" {
		var overrides map[string]interface{}
		if err := json.Unmarshal([]byte(form.EmailDelivery.OverridesJSON), &overrides); err == nil {
			if to, ok := overrides["to"].(string); ok {
				emailRecipient = to
			}
		}
	}

	endpoint := fmt.Sprintf("/forms/%s/submit", form.Slug)
	actionURL := liveFormAction(form.Slug, form.Token)
	captchaEmbed := buildCaptchaEmbed(form)
	formCode := ""
	hasGeneratedHTML := strings.TrimSpace(form.GeneratedHTML) != ""

	if hasGeneratedHTML {
		var prepared string
		var err error
		if prepared, err = normalizeFormHTML(form.GeneratedHTML, actionURL, form); err != nil {
			if logger != nil {
				logger.Warn("failed to normalize generated form HTML", zap.Error(err), zap.Uint("form_id", form.ID))
			}
			prepared = form.GeneratedHTML
		}
		formCode = injectCaptchaSnippet(prepared, captchaEmbed)
	} else {
		formCode = buildDefaultFormCode(actionURL, form, captchaEmbed)
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":            form.Name,
		"Form":             form,
		"Submissions":      submissions,
		"Endpoint":         endpoint,
		"Token":            form.Token,
		"WebhookEvents":    webhookEvents,
		"EmailEvents":      emailEvents,
		"EmailRecipient":   emailRecipient,
		"FormCode":         formCode,
		"HasGeneratedHTML": hasGeneratedHTML,
		"ContentView":      "admin/forms/show/content",
	}, "")
}

// AdminFormsEdit renders the edit form.
func AdminFormsEdit(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	form, err := forms.GetByID(db, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	// Initialize deliveries if they don't exist
	logger := ctx.Logger
	if err := forms.EnsureDeliveryRecords(logger, db, form); err != nil {
		logger.Error("failed to ensure delivery records", zap.Error(err))
	}

	// Load profiles for dropdowns
	mailerProfiles, _ := integrations.ListMailerProfiles(db)
	captchaProfiles, _ := integrations.ListCaptchaProfiles(db)

	// Extract email recipient from overrides for display
	emailRecipient := ""
	if form.EmailDelivery != nil && form.EmailDelivery.OverridesJSON != "" {
		var overrides map[string]interface{}
		if err := json.Unmarshal([]byte(form.EmailDelivery.OverridesJSON), &overrides); err == nil {
			if to, ok := overrides["to"].(string); ok {
				emailRecipient = to
			}
		}
	}

	// Extract selected profile IDs
	selectedMailerProfileID := uint(0)
	if form.EmailDelivery != nil && form.EmailDelivery.MailerProfileID != nil {
		selectedMailerProfileID = *form.EmailDelivery.MailerProfileID
	}

	selectedCaptchaProfileID := uint(0)
	if form.CaptchaProfileID != nil {
		selectedCaptchaProfileID = *form.CaptchaProfileID
	}

	previewHTML := form.GeneratedHTML
	if strings.TrimSpace(previewHTML) == "" {
		if blank := GetTemplateByID("blank"); blank != nil {
			previewHTML = blank.RenderHTML(liveFormAction(form.Slug, form.Token))
		}
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":                    "Edit Form",
		"Form":                     form,
		"EmailDelivery":            form.EmailDelivery,
		"WebhookDelivery":          form.WebhookDelivery,
		"EmailRecipient":           emailRecipient,
		"IsEdit":                   true,
		"MailerProfiles":           mailerProfiles,
		"CaptchaProfiles":          captchaProfiles,
		"SelectedMailerProfileID":  selectedMailerProfileID,
		"SelectedCaptchaProfileID": selectedCaptchaProfileID,
		"PreviewHTML":              previewHTML,
		"ContentView":              "admin/forms/new/content",
	}, "")
}

// AdminFormsUpdate persists changes to an existing form.
func AdminFormsUpdate(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	cfg := ctx.Config
	logger := ctx.Logger

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	// Parse mailer profile ID
	var mailerProfileID *uint
	if mailerIDStr := ctx.FormValue("mailer_profile_id"); mailerIDStr != "" {
		if pid, err := strconv.ParseUint(mailerIDStr, 10, 32); err == nil {
			uid := uint(pid)
			mailerProfileID = &uid
		}
	}

	// Parse captcha profile ID
	var captchaProfileID *uint
	if captchaIDStr := ctx.FormValue("captcha_profile_id"); captchaIDStr != "" {
		if pid, err := strconv.ParseUint(captchaIDStr, 10, 32); err == nil {
			uid := uint(pid)
			captchaProfileID = &uid
		}
	}

	params := forms.UpdateParams{
		ID:                 uint(id),
		Name:               ctx.FormValue("name"),
		AllowedOrigins:     ctx.FormValue("allowed_origins"),
		MailerProfileID:    mailerProfileID,
		CaptchaProfileID:   captchaProfileID,
		EmailRecipient:     ctx.FormValue("email_recipient"),
		EmailEnabled:       ctx.FormValue("email_enabled") == "on",
		WebhookEnabled:     ctx.FormValue("webhook_enabled") == "on",
		WebhookURL:         ctx.FormValue("webhook_url"),
		WebhookSecret:      ctx.FormValue("webhook_secret"),
		WebhookHeadersJSON: ctx.FormValue("webhook_headers"),
	}

	updatedForm, err := forms.Update(logger, db, params)
	if err != nil {
		// Handle validation errors
		if valErr, ok := err.(*forms.ValidationError); ok {
			form, _ := forms.GetByID(db, uint(id))
			return renderFormError(ctx, cfg, valErr.Message, form, form.EmailDelivery, form.WebhookDelivery, true, nil)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect(fmt.Sprintf("/admin/forms/%d", updatedForm.ID))
}

func renderFormError(ctx *cartridge.Context, cfg *config.Config, message string, form *forms.Form, emailDelivery *forms.EmailDelivery, webhookDelivery *forms.WebhookDelivery, isEdit bool, template *FormTemplate) error {
	// Load profiles for dropdowns
	db, _ := ctx.DB()
	mailerProfiles, _ := integrations.ListMailerProfiles(db)
	captchaProfiles, _ := integrations.ListCaptchaProfiles(db)

	// Extract email recipient from overrides for display
	emailRecipient := ""
	if emailDelivery != nil && emailDelivery.OverridesJSON != "" {
		var overrides map[string]interface{}
		if err := json.Unmarshal([]byte(emailDelivery.OverridesJSON), &overrides); err == nil {
			if to, ok := overrides["to"].(string); ok {
				emailRecipient = to
			}
		}
	}

	// Extract selected profile IDs
	selectedMailerProfileID := uint(0)
	if emailDelivery != nil && emailDelivery.MailerProfileID != nil {
		selectedMailerProfileID = *emailDelivery.MailerProfileID
	}

	selectedCaptchaProfileID := uint(0)
	if form != nil && form.CaptchaProfileID != nil {
		selectedCaptchaProfileID = *form.CaptchaProfileID
	}

	data := fiber.Map{
		"Title":                    "New Form",
		"Error":                    message,
		"DefaultSlug":              forms.Slugify("New Form"),
		"EmailDelivery":            emailDelivery,
		"WebhookDelivery":          webhookDelivery,
		"EmailRecipient":           emailRecipient,
		"IsEdit":                   isEdit,
		"MailerProfiles":           mailerProfiles,
		"CaptchaProfiles":          captchaProfiles,
		"SelectedMailerProfileID":  selectedMailerProfileID,
		"SelectedCaptchaProfileID": selectedCaptchaProfileID,
		"PreviewHTML":              "",
		"ContentView":              "admin/forms/new/content",
	}
	if form != nil {
		data["Form"] = form
		if isEdit {
			data["Title"] = "Edit Form"
			data["DefaultSlug"] = form.Slug
		}
	}
	if template != nil {
		data["Template"] = template
		data["TemplateID"] = template.ID
		data["FormName"] = template.Name
		if !isEdit && template.Slug != "" {
			data["DefaultSlug"] = template.Slug
		}
		if data["PreviewHTML"] == "" {
			data["PreviewHTML"] = template.RenderHTML(exampleFormAction(template.Slug))
		}
	}

	if form != nil && strings.TrimSpace(form.GeneratedHTML) != "" {
		data["PreviewHTML"] = form.GeneratedHTML
	}

	if preview, ok := data["PreviewHTML"].(string); ok && strings.TrimSpace(preview) == "" {
		if blank := GetTemplateByID("blank"); blank != nil {
			action := ""
			if form != nil {
				action = liveFormAction(form.Slug, form.Token)
			} else if template != nil {
				action = exampleFormAction(template.Slug)
			}
			data["PreviewHTML"] = blank.RenderHTML(action)
		}
	}

	return ctx.Render("layouts/base", data, "")
}

func buildDefaultFormCode(actionURL string, form *forms.Form, embed *captchaEmbed) string {
	if form == nil {
		return ""
	}

	publicID := strings.TrimSpace(form.PublicID)
	token := strings.TrimSpace(form.Token)

	publicAttr := ""
	if publicID != "" {
		publicAttr = fmt.Sprintf(` data-form-public-id="%s"`, publicID)
	}

	tokenAttr := ""
	if token != "" {
		tokenAttr = fmt.Sprintf(` data-form-token="%s"`, token)
	}

	baseForm := fmt.Sprintf(`<form action="%s" method="POST" data-form-id="%d"%s%s>
    <label>Name
        <input type="text" name="name" required>
    </label>

    <label>Email
        <input type="email" name="email" required>
    </label>

    <label>Message
        <textarea name="message" rows="4"></textarea>
    </label>

    <button type="submit">Send</button>
</form>`, htmlstd.EscapeString(actionURL), form.ID, publicAttr, tokenAttr)

	return injectCaptchaSnippet(baseForm, embed)
}

func normalizeFormHTML(rawHTML, actionURL string, form *forms.Form) (string, error) {
	if strings.TrimSpace(rawHTML) == "" {
		return "", errors.New("generated HTML is empty")
	}
	if form == nil {
		return rawHTML, errors.New("form context missing")
	}

	start, end := findFormTagBounds(rawHTML)
	if start == -1 || end == -1 {
		return rawHTML, errors.New("form tag not found in generated HTML")
	}

	opening := rawHTML[start : end+1]
	rewritten, err := rewriteOpeningFormTag(opening, actionURL, form)
	if err != nil {
		return rawHTML, err
	}

	return rawHTML[:start] + rewritten + rawHTML[end+1:], nil
}

func rewriteOpeningFormTag(tag, actionURL string, form *forms.Form) (string, error) {
	fragment := tag + "</form>"
	nodes, err := htmlnode.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return "", err
	}

	var formNode *htmlnode.Node
	for _, n := range nodes {
		formNode = findFormNode(n)
		if formNode != nil {
			break
		}
	}
	if formNode == nil {
		return "", errors.New("form node missing in fragment")
	}

	setOrAddAttr(formNode, "action", actionURL)
	setOrAddAttr(formNode, "method", "POST")
	setOrAddAttr(formNode, "data-form-id", fmt.Sprintf("%d", form.ID))
	if publicID := strings.TrimSpace(form.PublicID); publicID != "" {
		setOrAddAttr(formNode, "data-form-public-id", publicID)
	}
	if token := strings.TrimSpace(form.Token); token != "" {
		setOrAddAttr(formNode, "data-form-token", token)
	}

	return serializeOpeningTag(formNode), nil
}

func findFormNode(node *htmlnode.Node) *htmlnode.Node {
	if node == nil {
		return nil
	}
	if node.Type == htmlnode.ElementNode && strings.EqualFold(node.Data, "form") {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFormNode(child); found != nil {
			return found
		}
	}
	return nil
}

func setOrAddAttr(node *htmlnode.Node, key, value string) {
	if node == nil {
		return
	}
	for i := range node.Attr {
		attr := &node.Attr[i]
		if attr.Namespace == "" && strings.EqualFold(attr.Key, key) {
			attr.Key = key
			attr.Val = value
			return
		}
	}
	node.Attr = append(node.Attr, htmlnode.Attribute{Key: key, Val: value})
}

func serializeOpeningTag(node *htmlnode.Node) string {
	if node == nil {
		return "<form>"
	}
	var builder strings.Builder
	builder.Grow(64)
	builder.WriteString("<")
	builder.WriteString(node.Data)
	for _, attr := range node.Attr {
		if attr.Key == "" {
			continue
		}
		builder.WriteString(" ")
		if attr.Namespace != "" {
			builder.WriteString(attr.Namespace)
			builder.WriteString(":")
		}
		builder.WriteString(attr.Key)
		builder.WriteString(`="`)
		builder.WriteString(htmlstd.EscapeString(attr.Val))
		builder.WriteString(`"`)
	}
	builder.WriteString(">")
	return builder.String()
}

func findFormTagBounds(input string) (int, int) {
	lower := strings.ToLower(input)
	start := strings.Index(lower, "<form")
	if start == -1 {
		return -1, -1
	}

	var quote byte
	for i := start; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else if ch == '\\' && i+1 < len(input) {
				i++
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '>' {
			return start, i
		}
	}
	return start, -1
}

type captchaEmbed struct {
	WidgetMarkup string
	ScriptTag    string
}

func (c *captchaEmbed) isEmpty() bool {
	if c == nil {
		return true
	}
	return strings.TrimSpace(c.WidgetMarkup) == "" && strings.TrimSpace(c.ScriptTag) == ""
}

func injectCaptchaSnippet(html string, embed *captchaEmbed) string {
	if embed == nil || embed.isEmpty() {
		return html
	}

	widget := strings.TrimSpace(embed.WidgetMarkup)
	script := strings.TrimSpace(embed.ScriptTag)
	if widget == "" && script == "" {
		return html
	}

	lower := strings.ToLower(html)
	closeIdx := strings.Index(lower, "</form>")
	if closeIdx == -1 {
		var builder strings.Builder
		builder.Grow(len(html) + len(widget) + len(script) + 8)
		builder.WriteString(html)
		if widget != "" {
			if !strings.HasSuffix(html, "\n") {
				builder.WriteString("\n")
			}
			builder.WriteString(widget)
			builder.WriteString("\n")
		}
		if script != "" {
			if widget == "" && !strings.HasSuffix(html, "\n") {
				builder.WriteString("\n")
			}
			builder.WriteString(script)
		}
		return builder.String()
	}

	before := html[:closeIdx]
	after := html[closeIdx:]

	var builder strings.Builder
	builder.Grow(len(html) + len(widget) + len(script) + 8)
	builder.WriteString(before)
	if widget != "" {
		if !strings.HasSuffix(before, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString(widget)
		builder.WriteString("\n")
	}
	builder.WriteString(after)
	if script != "" {
		if !strings.HasSuffix(after, "\n") {
			builder.WriteString("\n")
		}
		builder.WriteString(script)
	}
	return builder.String()
}

func buildCaptchaEmbed(form *forms.Form) *captchaEmbed {
	if form == nil || form.CaptchaProfileID == nil || form.CaptchaProfile == nil {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(form.CaptchaProfile.Provider)) {
	case "turnstile":
		return buildTurnstileEmbed(form)
	default:
		return nil
	}
}

func buildTurnstileEmbed(form *forms.Form) *captchaEmbed {
	profile := form.CaptchaProfile
	policy := parseCaptchaPolicy(profile.PolicyJSON, form.CaptchaOverridesJSON)
	siteKey := strings.TrimSpace(policy.SiteKey)
	if siteKey == "" {
		siteKeys := parseCaptchaSiteKeys(profile.SiteKeysJSON)
		siteKey = selectCaptchaSiteKey(form, siteKeys)
	}
	if siteKey == "" {
		siteKey = "YOUR_TURNSTILE_SITE_KEY"
	}

	attrs := []string{fmt.Sprintf(`data-sitekey="%s"`, htmlstd.EscapeString(siteKey))}
	if policy.Action != "" {
		attrs = append(attrs, fmt.Sprintf(`data-action="%s"`, htmlstd.EscapeString(policy.Action)))
	}
	if policy.Theme != "" {
		attrs = append(attrs, fmt.Sprintf(`data-theme="%s"`, htmlstd.EscapeString(policy.Theme)))
	}
	if policy.Language != "" {
		attrs = append(attrs, fmt.Sprintf(`data-language="%s"`, htmlstd.EscapeString(policy.Language)))
	}
	if policy.Size != "" {
		attrs = append(attrs, fmt.Sprintf(`data-size="%s"`, htmlstd.EscapeString(policy.Size)))
	} else if strings.EqualFold(policy.Widget, "invisible") {
		attrs = append(attrs, `data-size="invisible"`)
	}

	widget := fmt.Sprintf(`    <div class="formlander-captcha-block">
        <div class="cf-turnstile" %s></div>
    </div>`, strings.Join(attrs, " "))

	script := `<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>`

	return &captchaEmbed{
		WidgetMarkup: widget,
		ScriptTag:    script,
	}
}

type captchaSiteKeyEntry struct {
	HostPattern string `json:"host_pattern"`
	SiteKey     string `json:"site_key"`
}

func parseCaptchaSiteKeys(raw string) []captchaSiteKeyEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var entries []captchaSiteKeyEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	return entries
}

type captchaPolicy struct {
	Action   string
	Theme    string
	Language string
	Widget   string
	Size     string
	Required bool
	SiteKey  string
}

func parseCaptchaPolicy(baseJSON, overrideJSON string) captchaPolicy {
	policy := captchaPolicy{
		Action: "submit",
		Theme:  "auto",
	}
	policy = applyPolicyJSON(policy, baseJSON)
	policy = applyPolicyJSON(policy, overrideJSON)
	return policy
}

func applyPolicyJSON(policy captchaPolicy, raw string) captchaPolicy {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return policy
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return policy
	}

	if v, ok := data["action"].(string); ok && strings.TrimSpace(v) != "" {
		policy.Action = v
	}
	if v, ok := data["theme"].(string); ok && strings.TrimSpace(v) != "" {
		policy.Theme = v
	}
	if v, ok := data["language"].(string); ok && strings.TrimSpace(v) != "" {
		policy.Language = v
	}
	if v, ok := data["widget"].(string); ok && strings.TrimSpace(v) != "" {
		policy.Widget = v
	}
	if v, ok := data["size"].(string); ok && strings.TrimSpace(v) != "" {
		policy.Size = v
	}
	if v, ok := data["required"].(bool); ok {
		policy.Required = v
	}
	if v, ok := data["site_key"].(string); ok && strings.TrimSpace(v) != "" {
		policy.SiteKey = v
	}

	return policy
}

func selectCaptchaSiteKey(form *forms.Form, entries []captchaSiteKeyEntry) string {
	if len(entries) == 0 {
		return ""
	}

	allowed := strings.TrimSpace(form.AllowedOrigins)
	var hosts []string
	if allowed != "" && allowed != "*" {
		for _, origin := range strings.Split(allowed, ",") {
			origin = strings.TrimSpace(origin)
			if origin == "" || strings.Contains(origin, "*") {
				continue
			}
			host := extractDomain(origin)
			if host == "" {
				host = extractDomain("https://" + origin)
			}
			if host != "" {
				hosts = append(hosts, host)
			}
		}
	}

	for _, host := range hosts {
		if key := findSiteKeyForHost(host, entries); strings.TrimSpace(key) != "" {
			return key
		}
	}

	for _, entry := range entries {
		if strings.TrimSpace(entry.HostPattern) == "*" && strings.TrimSpace(entry.SiteKey) != "" {
			return entry.SiteKey
		}
	}

	for _, entry := range entries {
		if strings.TrimSpace(entry.SiteKey) != "" {
			return entry.SiteKey
		}
	}

	return ""
}

func findSiteKeyForHost(host string, entries []captchaSiteKeyEntry) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	for _, entry := range entries {
		pattern := strings.ToLower(strings.TrimSpace(entry.HostPattern))
		if pattern == "" || strings.TrimSpace(entry.SiteKey) == "" {
			continue
		}
		if pattern == "*" {
			return entry.SiteKey
		}
		if pattern == host {
			return entry.SiteKey
		}
		if strings.HasPrefix(pattern, "*.") {
			base := strings.TrimPrefix(pattern, "*.")
			if host == base || strings.HasSuffix(host, "."+base) {
				return entry.SiteKey
			}
			continue
		}
		if strings.HasPrefix(pattern, ".") {
			base := strings.TrimPrefix(pattern, ".")
			if strings.HasSuffix(host, base) {
				return entry.SiteKey
			}
		}
	}
	return ""
}

func exampleFormAction(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = "your-form"
	}
	return fmt.Sprintf("/forms/%s/submit?token=YOUR_FORM_TOKEN", slug)
}

func liveFormAction(slug, token string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = "your-form"
	}
	token = strings.TrimSpace(token)
	if token == "" {
		token = "YOUR_FORM_TOKEN"
	}
	return fmt.Sprintf("/forms/%s/submit?token=%s", slug, token)
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}
