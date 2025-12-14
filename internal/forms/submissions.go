package forms

import (
	"encoding/json"
	"fmt"
	"time"

	"log/slog"
	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

// SubmissionParams holds parameters for creating a submission
type SubmissionParams struct {
	FormID    uint
	DataJSON  string
	UserAgent string
	IsSpam    bool
}

// CreateSubmission creates a new submission and associated delivery events
func CreateSubmission(logger *slog.Logger, db *gorm.DB, form *Form, payload map[string]any, userAgent string) (*Submission, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		logger.Error("encode submission payload", slog.Any("error", err))
		return nil, fmt.Errorf("failed to encode submission payload")
	}

	submission := &Submission{
		FormID:    form.ID,
		DataJSON:  string(encoded),
		IPHash:    "", // Not stored for privacy - only used for rate limiting
		UserAgent: userAgent,
		IsSpam:    false,
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		if err := tx.Create(submission).Error; err != nil {
			return err
		}

		// Check webhook delivery
		webhookDelivery := form.WebhookDelivery
		if webhookDelivery != nil && webhookDelivery.Enabled && webhookDelivery.URL != "" {
			event := NewWebhookEvent(submission.ID, time.Now().UTC())
			if err := tx.Create(event).Error; err != nil {
				return err
			}
		}

		// Check email delivery
		emailDelivery := form.EmailDelivery
		if emailDelivery != nil && emailDelivery.Enabled {
			recipient := extractEmailRecipient(emailDelivery)
			if recipient != "" {
				event := NewEmailEvent(submission.ID, time.Now().UTC())
				if err := tx.Create(event).Error; err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		logger.Error("store submission failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to save submission")
	}

	return submission, nil
}

// extractEmailRecipient extracts the recipient email from email delivery overrides
func extractEmailRecipient(emailDelivery *EmailDelivery) string {
	if emailDelivery == nil {
		return ""
	}
	// Extract recipient from overrides_json
	if emailDelivery.OverridesJSON != "" {
		var overrides map[string]interface{}
		if err := json.Unmarshal([]byte(emailDelivery.OverridesJSON), &overrides); err == nil {
			if to, ok := overrides["to"].(string); ok && to != "" {
				return to
			}
		}
	}
	return ""
}
