package jobs

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"log/slog"

	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
)

// WebhookDispatcher asynchronously delivers webhook events.
type WebhookDispatcher struct {
	cfg   *config.Config
	http  *http.Client
	retry *RetryStrategy
}

// NewWebhookDispatcher constructs a dispatcher with sane defaults.
func NewWebhookDispatcher(cfg *config.Config) *WebhookDispatcher {
	client := &http.Client{Timeout: 10 * time.Second}
	return &WebhookDispatcher{
		cfg:   cfg,
		http:  client,
		retry: NewRetryStrategy(cfg),
	}
}

// ProcessBatch implements the Processor interface.
func (d *WebhookDispatcher) ProcessBatch(ctx *JobContext) error {
	db := ctx.DB
	now := time.Now().UTC()
	var events []forms.WebhookEvent
	if err := db.
		Preload("Submission").
		Preload("Submission.Form.WebhookDelivery").
		Where("status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", []string{forms.WebhookStatusPending, forms.WebhookStatusRetrying}, now).
		Order("created_at ASC").
		Limit(10).
		Find(&events).Error; err != nil {
		ctx.Logger.Error("query pending webhooks", slog.Any("error", err))
		return err
	}

	if len(events) == 0 {
		return nil
	}

	for i := range events {
		d.handleEvent(ctx, db, &events[i])
	}

	return nil
}

func (d *WebhookDispatcher) handleEvent(ctx *JobContext, db *gorm.DB, event *forms.WebhookEvent) {
	if event.Submission == nil || event.Submission.Form == nil {
		// Ensure required associations are loaded.
		if err := db.
			Preload("Submission").
			Preload("Submission.Form").
			First(event, event.ID).Error; err != nil {
			ctx.Logger.Error("load webhook associations", slog.Uint64("id", uint64(event.ID)), slog.Any("error", err))
			return
		}
	}

	form := event.Submission.Form
	webhookDelivery := form.WebhookDelivery
	if webhookDelivery == nil || !webhookDelivery.Enabled || webhookDelivery.URL == "" {
		// Disable further attempts.
		MarkWebhookAsFinal(ctx, db, event, forms.WebhookStatusFailed, "webhooks disabled for form")
		return
	}

	body, err := d.buildPayload(event)
	if err != nil {
		MarkWebhookAsRetry(ctx, db, event, d.retry, err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookDelivery.URL, bytes.NewReader(body))
	if err != nil {
		MarkWebhookAsRetry(ctx, db, event, d.retry, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Formlander/1.0")

	for key, value := range d.resolveHeaders(ctx, webhookDelivery) {
		req.Header.Set(key, value)
	}

	if webhookDelivery.Secret != "" {
		signature := computeSignature(body, webhookDelivery.Secret)
		req.Header.Set(d.cfg.Webhook.SignatureHeader, signature)
	}

	start := time.Now()
	resp, err := d.http.Do(req)
	if err != nil {
		MarkWebhookAsRetry(ctx, db, event, d.retry, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		MarkWebhookAsRetry(ctx, db, event, d.retry, fmt.Errorf("unexpected status %d", resp.StatusCode))
		return
	}

	// Success
	updater := NewEventUpdater(&forms.WebhookEvent{})
	attemptCount := event.AttemptCount + 1
	if err := updater.Update(ctx, db, event.ID, forms.WebhookStatusDelivered, start, "", WithAttemptCount(attemptCount), WithNextAttempt(nil)); err != nil {
		ctx.Logger.Error("update webhook event", slog.Uint64("id", uint64(event.ID)), slog.Any("error", err))
	} else {
		event.Status = forms.WebhookStatusDelivered
		event.AttemptCount = attemptCount
		last := start.UTC()
		event.LastAttemptAt = &last
		event.LastAttemptErr = ""
		event.NextAttemptAt = nil
	}
}

func (d *WebhookDispatcher) buildPayload(event *forms.WebhookEvent) ([]byte, error) {
	var submissionData any
	if err := json.Unmarshal([]byte(event.Submission.DataJSON), &submissionData); err != nil {
		submissionData = event.Submission.DataJSON
	}

	payload := map[string]any{
		"form": map[string]any{
			"id":         event.Submission.Form.ID,
			"public_id":  event.Submission.Form.PublicID,
			"name":       event.Submission.Form.Name,
			"slug":       event.Submission.Form.Slug,
			"created_at": event.Submission.Form.CreatedAt.UTC(),
		},
		"submission": map[string]any{
			"id":          event.Submission.ID,
			"data":        submissionData,
			"received_at": event.Submission.CreatedAt.UTC(),
			"user_agent":  event.Submission.UserAgent,
		},
	}

	return json.Marshal(payload)
}

func (d *WebhookDispatcher) resolveHeaders(ctx *JobContext, webhookDelivery *forms.WebhookDelivery) map[string]string {
	if webhookDelivery == nil || webhookDelivery.HeadersJSON == "" {
		return map[string]string{}
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(webhookDelivery.HeadersJSON), &headers); err != nil {
		ctx.Logger.Warn("invalid webhook headers JSON", slog.Uint64("form_id", uint64(webhookDelivery.FormID)), slog.Any("error", err))
		return map[string]string{}
	}
	return headers
}

func computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
