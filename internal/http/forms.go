package http

import (
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"log/slog"
	"strconv"
	"strings"

	"github.com/karloscodes/cartridge"
	htmlnode "golang.org/x/net/html"
	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/integrations"
)

// AdminFormsIndex renders the list of forms.
func AdminFormsIndex(ctx *cartridge.Context) error {
	db := ctx.DB()

	formsList, err := forms.List(db)
	if err != nil {
		return cartridge.NewError(500)
	}

	// Load relations for display
	for i := range formsList {
		db.Preload("EmailDelivery").Preload("WebhookDelivery").First(&formsList[i], formsList[i].ID)
	}

	// How many submissions each form has.
	var rows []struct {
		FormID uint
		Count  int64
	}
	db.Model(&forms.Submission{}).Select("form_id, COUNT(*) AS count").Group("form_id").Scan(&rows)
	counts := make(map[uint]int64, len(formsList))
	for _, form := range formsList {
		counts[form.ID] = 0
	}
	for _, row := range rows {
		counts[row.FormID] = row.Count
	}

	return ctx.Render("layouts/base", cartridge.Map{
		"Title":       "Forms",
		"Forms":       formsList,
		"Counts":      counts,
		"CreateRoute": "/admin/forms/new",
		"ContentView": "admin/forms/index/content",
	}, "")
}

// formInput is what the form screen shows in its fields: the defaults of a
// starter template, the saved form, or what the owner typed before an error.
type formInput struct {
	Name             string
	Slug             string
	AllowedOrigins   string
	UseSDK           bool
	CaptchaProfileID uint
	EmailEnabled     bool
	MailerProfileID  uint
	EmailRecipient   string
	EmailSubject     string
	WebhookEnabled   bool
	WebhookProfileID uint
}

// postedProfileID reads the ID of a profile from a select. An empty select
// gives nil.
func postedProfileID(ctx *cartridge.Context, field string) *uint {
	id, err := strconv.ParseUint(ctx.FormValue(field), 10, 32)
	if err != nil || id == 0 {
		return nil
	}
	uid := uint(id)
	return &uid
}

func profileID(id *uint) uint {
	if id == nil {
		return 0
	}
	return *id
}

// formInputFromPost reads the fields of the form screen from the request.
func formInputFromPost(ctx *cartridge.Context) formInput {
	return formInput{
		Name:             ctx.FormValue("name"),
		Slug:             ctx.FormValue("slug"),
		AllowedOrigins:   ctx.FormValue("allowed_origins"),
		UseSDK:           ctx.FormValue("use_sdk") == "on",
		CaptchaProfileID: profileID(postedProfileID(ctx, "captcha_profile_id")),
		EmailEnabled:     ctx.FormValue("email_enabled") == "on",
		MailerProfileID:  profileID(postedProfileID(ctx, "mailer_profile_id")),
		EmailRecipient:   ctx.FormValue("email_recipient"),
		EmailSubject:     ctx.FormValue("email_subject"),
		WebhookEnabled:   ctx.FormValue("webhook_enabled") == "on",
		WebhookProfileID: profileID(postedProfileID(ctx, "webhook_profile_id")),
	}
}

// formInputFromForm reads the fields of the form screen from a saved form.
func formInputFromForm(form *forms.Form) formInput {
	input := formInput{
		Name:             form.Name,
		Slug:             form.Slug,
		AllowedOrigins:   form.AllowedOrigins,
		UseSDK:           form.UseSDK,
		CaptchaProfileID: profileID(form.CaptchaProfileID),
	}
	if email := form.EmailDelivery; email != nil {
		input.EmailEnabled = email.Enabled
		input.MailerProfileID = profileID(email.MailerProfileID)
		overrides := email.Overrides()
		input.EmailRecipient = overrides.To
		input.EmailSubject = overrides.Subject
	}
	if webhook := form.WebhookDelivery; webhook != nil {
		input.WebhookEnabled = webhook.Enabled
		input.WebhookProfileID = profileID(webhook.WebhookProfileID)
	}
	return input
}

// renderFormScreen shows the screen that makes or edits a form. form is nil
// for a new form.
func renderFormScreen(ctx *cartridge.Context, form *forms.Form, templateID string, input formInput, message string) error {
	db := ctx.DB()
	mailerProfiles, _ := integrations.ListMailerProfiles(db)
	captchaProfiles, _ := integrations.ListCaptchaProfiles(db)
	webhookProfiles, _ := integrations.ListWebhookProfiles(db)

	data := cartridge.Map{
		"Title":           "New Form",
		"Error":           message,
		"Input":           input,
		"TemplateID":      templateID,
		"MailerProfiles":  mailerProfiles,
		"CaptchaProfiles": captchaProfiles,
		"WebhookProfiles": webhookProfiles,
		"ContentView":     "admin/forms/new/content",
	}
	// The example on the right: the code of the form, or of its starter.
	if form != nil {
		data["Title"] = "Edit Form"
		data["IsEdit"] = true
		data["Form"] = form
		_, data["FormCode"] = formCodeFor(ctx, form)
	} else if starter := GetTemplateByID(templateID); starter != nil {
		data["FormCode"] = starter.RenderHTML("")
	}
	return ctx.Render("layouts/base", data, "")
}

// AdminFormsNew renders the new form view or template selector.
func AdminFormsNew(ctx *cartridge.Context) error {
	// Check if a template is selected
	templateID := ctx.Query("template")
	if templateID == "" {
		// Show template selector
		return ctx.Render("layouts/base", cartridge.Map{
			"Title":       "Choose a Template",
			"Templates":   GetFormTemplates(),
			"ContentView": "admin/forms/templates/content",
		}, "")
	}

	template := GetTemplateByID(templateID)
	if template == nil {
		return ctx.Redirect("/admin/forms/new")
	}

	// A starter template sends email by default. That needs a mailer
	// profile, so the box starts off when there is none.
	mailerProfiles, _ := integrations.ListMailerProfiles(ctx.DB())
	input := formInput{
		Name:           template.Name,
		Slug:           template.Slug,
		UseSDK:         true,
		EmailEnabled:   template.EmailDelivery.Enabled && len(mailerProfiles) > 0,
		EmailRecipient: adminEmail(ctx),
	}
	if len(mailerProfiles) == 1 {
		input.MailerProfileID = mailerProfiles[0].ID
	}

	return renderFormScreen(ctx, nil, template.ID, input, "")
}

// AdminFormsCreate persists a new form configuration.
func AdminFormsCreate(ctx *cartridge.Context) error {
	db := ctx.DB()

	templateID := strings.TrimSpace(ctx.FormValue("template_id"))
	selectedTemplate := GetTemplateByID(templateID)

	// Use forms context for business logic
	params := forms.CreateParams{
		Name:             ctx.FormValue("name"),
		Slug:             ctx.FormValue("slug"),
		AllowedOrigins:   ctx.FormValue("allowed_origins"),
		ServerHost:       ctx.Hostname(),
		UseSDK:           ctx.FormValue("use_sdk") == "on",
		GeneratedHTML:    ctx.FormValue("generated_html"),
		MailerProfileID:  postedProfileID(ctx, "mailer_profile_id"),
		CaptchaProfileID: postedProfileID(ctx, "captcha_profile_id"),
		EmailRecipient:   ctx.FormValue("email_recipient"),
		EmailSubject:     ctx.FormValue("email_subject"),
		EmailEnabled:     ctx.FormValue("email_enabled") == "on",
		WebhookEnabled:   ctx.FormValue("webhook_enabled") == "on",
		WebhookProfileID: postedProfileID(ctx, "webhook_profile_id"),
		TemplateID:       templateID,
	}

	form, err := forms.Create(ctx.Logger, db, params)
	if err != nil {
		// Handle validation errors
		if validationErr, ok := err.(*forms.ValidationError); ok {
			return renderFormScreen(ctx, nil, templateID, formInputFromPost(ctx), validationErr.Message)
		}
		ctx.Logger.Error("failed to create form", slog.Any("error", err))
		return cartridge.NewError(500)
	}

	// Update generated HTML if template was selected
	if selectedTemplate != nil {
		if html := selectedTemplate.RenderHTML(liveFormAction(form.Slug, form.Token)); strings.TrimSpace(html) != "" {
			form.GeneratedHTML = html
			if err := db.Transaction(func(tx *gorm.DB) error {
				return tx.Model(form).Update("generated_html", html).Error
			}); err != nil {
				ctx.Logger.Error("failed to update generated HTML", slog.Any("error", err))
			}
		}
	}

	return ctx.Redirect(fmt.Sprintf("/admin/forms/%d", form.ID))
}

// AdminFormShow displays a form summary and recent submissions.
func AdminFormShow(ctx *cartridge.Context) error {
	db := ctx.DB()
	logger := ctx.Logger

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return cartridge.NewError(404)
	}

	form, err := forms.GetByID(db, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cartridge.NewError(404)
		}
		return cartridge.NewError(500)
	}

	// Ensure delivery records exist
	if err := forms.EnsureDeliveryRecords(logger, db, form); err != nil {
		// Log but don't fail - continue showing the form
		logger.Error("failed to ensure delivery records", slog.Any("error", err))
	}

	submissions, err := forms.GetSubmissions(db, form.ID, 25)
	if err != nil {
		return cartridge.NewError(500)
	}

	webhookEvents, err := forms.GetWebhookEvents(db, form.ID, 20)
	if err != nil {
		return cartridge.NewError(500)
	}

	emailEvents, err := forms.GetEmailEvents(db, form.ID, 20)
	if err != nil {
		return cartridge.NewError(500)
	}

	endpoint := fmt.Sprintf("/forms/%s/submit", form.Slug)
	actionURL, formCode := formCodeFor(ctx, form)

	return ctx.Render("layouts/base", cartridge.Map{
		"Title":          form.Name,
		"Form":           form,
		"Submissions":    submissions,
		"ReturnTo":       fmt.Sprintf("/admin/forms/%d", form.ID),
		"Endpoint":       endpoint,
		"ActionURL":      actionURL,
		"CaptchaSiteKey": captchaSiteKey(form),
		"Token":          form.Token,
		"WebhookEvents":  webhookEvents,
		"EmailEvents":    emailEvents,
		"EmailRecipient": form.EmailDelivery.Overrides().To,
		"FormCode":       formCode,
		"CodeSplit":      true,
		"UseSDK":         form.UseSDK,
		"ContentView":    "admin/forms/show/content",
	}, "")
}

// formCodeFor returns the address a form posts to and the HTML to paste into
// a site. The code is pasted on other sites, so the address is absolute.
func formCodeFor(ctx *cartridge.Context, form *forms.Form) (actionURL, code string) {
	actionURL = ctx.BaseURL() + liveFormAction(form.Slug, form.Token)
	embed := buildCaptchaEmbed(form)
	if strings.TrimSpace(form.GeneratedHTML) == "" {
		return actionURL, buildDefaultFormCode(actionURL, form, embed)
	}
	prepared, err := normalizeFormHTML(form.GeneratedHTML, actionURL, form)
	if err != nil {
		ctx.Logger.Warn("failed to normalize generated form HTML", slog.Any("error", err), slog.Uint64("form_id", uint64(form.ID)))
		prepared = form.GeneratedHTML
	}
	return actionURL, injectCaptchaSnippet(prepared, embed)
}

// AdminFormsEdit renders the edit form.
func AdminFormsEdit(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return cartridge.NewError(404)
	}

	form, err := forms.GetByID(db, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cartridge.NewError(404)
		}
		return cartridge.NewError(500)
	}

	// Initialize deliveries if they don't exist
	if err := forms.EnsureDeliveryRecords(ctx.Logger, db, form); err != nil {
		ctx.Logger.Error("failed to ensure delivery records", slog.Any("error", err))
	}

	return renderFormScreen(ctx, form, "", formInputFromForm(form), "")
}

// AdminFormsUpdate persists changes to an existing form.
func AdminFormsUpdate(ctx *cartridge.Context) error {
	db := ctx.DB()
	logger := ctx.Logger

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return cartridge.NewError(404)
	}

	params := forms.UpdateParams{
		ID:               uint(id),
		Name:             ctx.FormValue("name"),
		AllowedOrigins:   ctx.FormValue("allowed_origins"),
		ServerHost:       ctx.Hostname(),
		UseSDK:           ctx.FormValue("use_sdk") == "on",
		MailerProfileID:  postedProfileID(ctx, "mailer_profile_id"),
		CaptchaProfileID: postedProfileID(ctx, "captcha_profile_id"),
		EmailRecipient:   ctx.FormValue("email_recipient"),
		EmailSubject:     ctx.FormValue("email_subject"),
		EmailEnabled:     ctx.FormValue("email_enabled") == "on",
		WebhookEnabled:   ctx.FormValue("webhook_enabled") == "on",
		WebhookProfileID: postedProfileID(ctx, "webhook_profile_id"),
	}

	updatedForm, err := forms.Update(logger, db, params)
	if err != nil {
		// Handle validation errors
		if valErr, ok := err.(*forms.ValidationError); ok {
			form, loadErr := forms.GetByID(db, uint(id))
			if loadErr != nil {
				return cartridge.NewError(404)
			}
			input := formInputFromPost(ctx)
			input.Slug = form.Slug
			return renderFormScreen(ctx, form, "", input, valErr.Message)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cartridge.NewError(404)
		}
		return cartridge.NewError(500)
	}

	return ctx.Redirect(fmt.Sprintf("/admin/forms/%d", updatedForm.ID))
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

// captchaSiteKey returns the public site key that the captcha of a form uses
// on its allowed sites, or "" when the form has no captcha or no key fits.
func captchaSiteKey(form *forms.Form) string {
	if form == nil || form.CaptchaProfile == nil {
		return ""
	}
	policy := parseCaptchaPolicy(form.CaptchaProfile.PolicyJSON, form.CaptchaOverridesJSON)
	if key := strings.TrimSpace(policy.SiteKey); key != "" {
		return key
	}
	return selectCaptchaSiteKey(form, parseCaptchaSiteKeys(form.CaptchaProfile.SiteKeysJSON))
}

func buildTurnstileEmbed(form *forms.Form) *captchaEmbed {
	profile := form.CaptchaProfile
	policy := parseCaptchaPolicy(profile.PolicyJSON, form.CaptchaOverridesJSON)
	siteKey := captchaSiteKey(form)
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
