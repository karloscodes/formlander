package jobs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/cartridge/jobs"
)

// EmailDispatcher delivers form submissions via Mailgun.
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

// ProcessBatch implements the jobs.Processor interface.
func (d *EmailDispatcher) ProcessBatch(ctx *jobs.JobContext) error {
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
		ctx.Logger.Error("query email events", zap.Error(err))
		return err
	}

	for i := range events {
		d.handleEvent(ctx, db, &events[i])
	}

	return nil
}

func (d *EmailDispatcher) handleEvent(ctx *jobs.JobContext, db *gorm.DB, event *forms.EmailEvent) {
	if event.Submission == nil || event.Submission.Form == nil {
		if err := db.
			Preload("Submission").
			Preload("Submission.Form").
			First(event, event.ID).Error; err != nil {
			ctx.Logger.Error("load email event associations", zap.Uint("id", event.ID), zap.Error(err))
			return
		}
	}

	form := event.Submission.Form
	emailDelivery := form.EmailDelivery
	if emailDelivery == nil || !emailDelivery.Enabled {
		MarkEmailAsFinal(ctx, db, event, forms.WebhookStatusFailed, "email forwarding disabled")
		return
	}

	cfg := d.resolveMailgunConfig(ctx, emailDelivery)
	if cfg == nil {
		MarkEmailAsFinal(ctx, db, event, forms.WebhookStatusFailed, "mailgun configuration missing")
		return
	}

	values, err := d.buildFormBody(event, cfg)
	if err != nil {
		MarkEmailAsRetry(ctx, db, event, d.retry, err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint(), strings.NewReader(values.Encode()))
	if err != nil {
		MarkEmailAsRetry(ctx, db, event, d.retry, err)
		return
	}

	req.SetBasicAuth("api", cfg.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Formlander/1.0")

	resp, err := d.http.Do(req)
	if err != nil {
		MarkEmailAsRetry(ctx, db, event, d.retry, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		MarkEmailAsRetry(ctx, db, event, d.retry, fmt.Errorf("unexpected status %d", resp.StatusCode))
		return
	}

	updater := NewEventUpdater(&forms.EmailEvent{})
	attemptCount := event.AttemptCount + 1
	if err := updater.Update(ctx, db, event.ID, forms.WebhookStatusDelivered, time.Now(), "", WithAttemptCount(attemptCount), WithNextAttempt(nil)); err != nil {
		ctx.Logger.Error("update email event", zap.Uint("id", event.ID), zap.Error(err))
	} else {
		event.Status = forms.WebhookStatusDelivered
		event.AttemptCount = attemptCount
		last := time.Now().UTC()
		event.LastAttemptAt = &last
		event.LastAttemptErr = ""
		event.NextAttemptAt = nil
	}
}

type mailgunConfig struct {
	APIKey string
	Domain string
	From   string
	To     string
}

func (m *mailgunConfig) Endpoint() string {
	return fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", m.Domain)
}

func (d *EmailDispatcher) resolveMailgunConfig(ctx *jobs.JobContext, emailDelivery *forms.EmailDelivery) *mailgunConfig {
	db := ctx.DB

	// Resolve recipient from overrides_json or fallback
	var to string
	if emailDelivery.OverridesJSON != "" {
		var overrides map[string]interface{}
		if err := json.Unmarshal([]byte(emailDelivery.OverridesJSON), &overrides); err == nil {
			if toField, ok := overrides["to"].(string); ok && toField != "" {
				to = toField
			}
		}
	}

	// If no mailer profile, can't send email
	if emailDelivery.MailerProfileID == nil {
		return nil
	}

	// Load the mailer profile
	var profile integrations.MailerProfile
	if err := db.First(&profile, *emailDelivery.MailerProfileID).Error; err != nil {
		return nil
	}

	apiKey := profile.APIKey
	domain := profile.Domain
	from := profile.DefaultFromEmail
	if profile.DefaultFromName != "" {
		from = fmt.Sprintf("%s <%s>", profile.DefaultFromName, profile.DefaultFromEmail)
	}

	// Ensure we have all required config
	if apiKey == "" || domain == "" || from == "" || to == "" {
		return nil
	}

	return &mailgunConfig{
		APIKey: apiKey,
		Domain: domain,
		From:   from,
		To:     to,
	}
}

func (d *EmailDispatcher) buildFormBody(event *forms.EmailEvent, cfg *mailgunConfig) (url.Values, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.Submission.DataJSON), &payload); err != nil {
		payload = map[string]any{"raw": event.Submission.DataJSON}
	}

	subject := fmt.Sprintf("New submission · %s", event.Submission.Form.Name)
	body := renderEmailBody(payload)

	values := url.Values{}
	values.Set("from", cfg.From)
	values.Set("to", cfg.To)
	values.Set("subject", subject)
	values.Set("text", body)

	return values, nil
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
