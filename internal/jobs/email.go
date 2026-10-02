package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log/slog"

	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
)

// mailgunAPI is the address of the Mailgun HTTP API.
var mailgunAPI = "https://api.mailgun.net"

// EmailDispatcher delivers form submissions by SMTP or by Mailgun.
type EmailDispatcher struct {
	cfg   *config.Config
	http  *http.Client
	retry *RetryStrategy
}

// NewEmailDispatcher constructs a dispatcher for email forwarding.
func NewEmailDispatcher(cfg *config.Config) *EmailDispatcher {
	client := &http.Client{Timeout: 15 * time.Second}
	return &EmailDispatcher{
		cfg:   cfg,
		http:  client,
		retry: NewRetryStrategy(cfg),
	}
}

// ProcessBatch implements the Processor interface.
func (d *EmailDispatcher) ProcessBatch(ctx *JobContext) error {
	db := ctx.DB
	now := time.Now().UTC()
	var events []forms.EmailEvent
	if err := db.
		Preload("Submission").
		Preload("Submission.Form.EmailDelivery").
		Preload("Submission.Form.EmailDelivery.MailerProfile").
		Where("status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", []string{forms.WebhookStatusPending, forms.WebhookStatusRetrying}, now).
		Order("created_at ASC").
		Limit(10).
		Find(&events).Error; err != nil {
		ctx.Logger.Error("query email events", slog.Any("error", err))
		return err
	}

	for i := range events {
		d.handleEvent(ctx, db, &events[i])
	}

	return nil
}

func (d *EmailDispatcher) handleEvent(ctx *JobContext, db *gorm.DB, event *forms.EmailEvent) {
	if event.Submission == nil || event.Submission.Form == nil {
		if err := db.
			Preload("Submission").
			Preload("Submission.Form").
			First(event, event.ID).Error; err != nil {
			ctx.Logger.Error("load email event associations", slog.Uint64("id", uint64(event.ID)), slog.Any("error", err))
			return
		}
	}

	form := event.Submission.Form
	emailDelivery := form.EmailDelivery
	if emailDelivery == nil || !emailDelivery.Enabled {
		MarkEmailAsFinal(ctx, db, event, forms.WebhookStatusFailed, "email forwarding disabled")
		return
	}

	profile := loadMailerProfile(db, emailDelivery)
	to := emailDelivery.Overrides().To
	if profile == nil || to == "" {
		MarkEmailAsFinal(ctx, db, event, forms.WebhookStatusFailed, "the form has no mailer profile or no recipient")
		return
	}

	err := d.send(ctx, profile, message{
		To:      to,
		ReplyTo: replyTo(event.Submission),
		Subject: fmt.Sprintf("New submission · %s", event.Submission.Form.Name),
		Body:    bodyForEvent(event),
	})

	var incomplete configError
	if errors.As(err, &incomplete) {
		MarkEmailAsFinal(ctx, db, event, forms.WebhookStatusFailed, incomplete.Error())
		return
	}
	if err != nil {
		MarkEmailAsRetry(ctx, db, event, d.retry, err)
		return
	}

	d.markEmailDelivered(ctx, db, event)
}

// configError is a mailer profile that cannot send. A retry does not help.
type configError string

func (e configError) Error() string { return string(e) }

// replyTo returns the address a reply to the email goes to: the person who
// sent the submission. An address outside ASCII is left out, because a mail
// server without SMTPUTF8 can refuse the whole email for it.
func replyTo(submission *forms.Submission) string {
	address := submission.ReplyAddress()
	for i := 0; i < len(address); i++ {
		if address[i] >= 0x80 {
			return ""
		}
	}
	return address
}

// send delivers one message through the provider of a mailer profile. The
// profile gives the From address.
func (d *EmailDispatcher) send(ctx context.Context, profile *integrations.MailerProfile, m message) error {
	if profile.DefaultFromEmail == "" {
		return configError("the mailer profile has no From address")
	}
	m.From = profile.DefaultFromEmail
	if profile.DefaultFromName != "" {
		m.From = fmt.Sprintf("%s <%s>", profile.DefaultFromName, profile.DefaultFromEmail)
	}

	switch profile.Provider {
	case "mailgun":
		if profile.APIKey == "" || profile.Domain == "" {
			return configError("the mailer profile has no Mailgun API key or no domain")
		}
		return d.sendMailgun(ctx, profile, m)
	default: // smtp is the default provider
		cfg := smtpConfigFromProfile(profile, m.From, m.To)
		if cfg == nil {
			return configError("the mailer profile has no SMTP host or no port")
		}
		return sendSMTP(cfg, buildSMTPMessage(m))
	}
}

// SendTest sends a test email through a mailer profile, the same way a
// submission goes out.
func (d *EmailDispatcher) SendTest(ctx context.Context, profile *integrations.MailerProfile, to string) error {
	return d.send(ctx, profile, message{
		To:      to,
		Subject: "Test email from Formlander",
		Body:    fmt.Sprintf("This is a test from the mailer profile %q in Formlander.\n\nIf you can read it, the profile can send email.\n", profile.Name),
	})
}

// loadMailerProfile loads the mailer profile of a form, or nil when the form
// has none.
func loadMailerProfile(db *gorm.DB, emailDelivery *forms.EmailDelivery) *integrations.MailerProfile {
	if emailDelivery.MailerProfileID == nil {
		return nil
	}

	var profile integrations.MailerProfile
	if err := db.First(&profile, *emailDelivery.MailerProfileID).Error; err != nil {
		return nil
	}
	return &profile
}

// smtpConfigFromProfile builds an SMTP send config from a mailer profile,
// returning nil when required fields are missing.
func smtpConfigFromProfile(profile *integrations.MailerProfile, from, to string) *smtpConfig {
	if profile.SMTPHost == "" || profile.SMTPPort == 0 {
		return nil
	}
	encryption := profile.SMTPEncryption
	if encryption == "" {
		encryption = "starttls"
	}
	return &smtpConfig{
		Host:       profile.SMTPHost,
		Port:       profile.SMTPPort,
		Username:   profile.SMTPUsername,
		Password:   profile.SMTPPassword,
		Encryption: encryption,
		From:       from,
		To:         to,
	}
}

// sendMailgun delivers the message through the Mailgun HTTP API.
func (d *EmailDispatcher) sendMailgun(ctx context.Context, profile *integrations.MailerProfile, m message) error {
	values := url.Values{}
	values.Set("from", m.From)
	values.Set("to", m.To)
	if m.ReplyTo != "" {
		values.Set("h:Reply-To", m.ReplyTo)
	}
	values.Set("subject", m.Subject)
	values.Set("text", m.Body)

	endpoint := fmt.Sprintf("%s/v3/%s/messages", mailgunAPI, profile.Domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth("api", profile.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Formlander/1.0")

	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// markEmailDelivered records a successful send on the event.
func (d *EmailDispatcher) markEmailDelivered(ctx *JobContext, db *gorm.DB, event *forms.EmailEvent) {
	updater := NewEventUpdater(&forms.EmailEvent{})
	attemptCount := event.AttemptCount + 1
	if err := updater.Update(ctx, db, event.ID, forms.WebhookStatusDelivered, time.Now(), "", WithAttemptCount(attemptCount), WithNextAttempt(nil)); err != nil {
		ctx.Logger.Error("update email event", slog.Uint64("id", uint64(event.ID)), slog.Any("error", err))
		return
	}
	event.Status = forms.WebhookStatusDelivered
	event.AttemptCount = attemptCount
	last := time.Now().UTC()
	event.LastAttemptAt = &last
	event.LastAttemptErr = ""
	event.NextAttemptAt = nil
}

func bodyForEvent(event *forms.EmailEvent) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.Submission.DataJSON), &payload); err != nil {
		payload = map[string]any{"raw": event.Submission.DataJSON}
	}
	return renderEmailBody(payload)
}

func renderEmailBody(payload map[string]any) string {
	lines := make([]string, 0, len(payload)*2+2)
	lines = append(lines, "New submission received:")
	lines = append(lines, "")

	for key, value := range payload {
		serialized, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			serialized = []byte(fmt.Sprintf("%v", value))
		}
		lines = append(lines, fmt.Sprintf("%s:\n%s", key, string(serialized)))
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
