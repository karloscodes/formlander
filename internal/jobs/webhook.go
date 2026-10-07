package jobs

import (
	"bytes"
	"context"
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
	"formlander/internal/integrations"
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
	now := time.Now().UTC()
	events, err := d.due(ctx.DB, now)
	if err != nil {
		ctx.Logger.Error("query pending webhooks", slog.Any("error", err))
		return err
	}
	d.deliver(ctx, ctx.DB, events, now)
	return nil
}

// due returns up to 10 webhooks to send now, oldest first.
func (d *WebhookDispatcher) due(db *gorm.DB, now time.Time) ([]forms.WebhookEvent, error) {
	var events []forms.WebhookEvent
	err := db.
		Preload("Submission").
		Preload("Submission.Form.WebhookDelivery.WebhookProfile").
		Where("status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", []string{forms.WebhookStatusPending, forms.WebhookStatusRetrying}, now).
		Order("created_at ASC").
		Limit(10).
		Find(&events).Error
	return events, err
}

// deliver claims each webhook and sends the ones this process claimed.
func (d *WebhookDispatcher) deliver(ctx *JobContext, db *gorm.DB, events []forms.WebhookEvent, now time.Time) {
	for i := range events {
		claimed, err := claim(db, &forms.WebhookEvent{}, events[i].ID, now)
		if err != nil {
			ctx.Logger.Error("claim webhook event", slog.Uint64("id", uint64(events[i].ID)), slog.Any("error", err))
			continue
		}
		if !claimed {
			continue // another process sends it
		}
		d.handleEvent(ctx, db, &events[i])
	}
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

	delivery := event.Submission.Form.WebhookDelivery
	if !delivery.Delivers() || delivery.WebhookProfile == nil {
		// Disable further attempts.
		MarkWebhookAsFinal(ctx, db, event, forms.WebhookStatusFailed, "webhooks disabled for form")
		return
	}

	body, err := d.buildPayload(event)
	if err != nil {
		MarkWebhookAsRetry(ctx, db, event, d.retry, err)
		return
	}

	start := time.Now()
	status, err := d.post(ctx, delivery.WebhookProfile, body)
	if err != nil {
		MarkWebhookAsRetry(ctx, db, event, d.retry, err)
		return
	}
	if status < 200 || status >= 300 {
		MarkWebhookAsRetry(ctx, db, event, d.retry, fmt.Errorf("unexpected status %d", status))
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

// post sends one JSON body to the URL of a webhook profile, with the headers
// and the signature of the profile. It returns the HTTP status of the answer.
func (d *WebhookDispatcher) post(ctx context.Context, profile *integrations.WebhookProfile, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, profile.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Formlander/1.0")
	for _, header := range profile.Headers() {
		req.Header.Set(header.Name, header.Value)
	}
	if profile.Secret != "" {
		req.Header.Set(d.cfg.Webhook.SignatureHeader, computeSignature(body, profile.Secret))
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// SendTest posts a sample submission to a webhook profile, the same way a
// real submission goes out, and returns the HTTP status of the answer.
func (d *WebhookDispatcher) SendTest(ctx context.Context, profile *integrations.WebhookProfile) (int, error) {
	now := time.Now().UTC()
	body, err := json.Marshal(map[string]any{
		"test": true,
		"form": map[string]any{
			"id":         0,
			"public_id":  "test",
			"name":       "Test from Formlander",
			"slug":       "test",
			"created_at": now,
		},
		"submission": map[string]any{
			"id": 0,
			"data": map[string]any{
				"name":    "Ada Lovelace",
				"email":   "ada@example.com",
				"message": "This is a test from Formlander. No person sent it.",
			},
			"received_at": now,
			"user_agent":  "Formlander/1.0",
		},
	})
	if err != nil {
		return 0, err
	}
	return d.post(ctx, profile, body)
}

func computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
