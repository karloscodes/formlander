package forms

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

// HoneypotField is the form field name reserved for bot-trap detection.
// Real users leave it empty; bots that fill out every field expose themselves.
// Submissions where this field has a non-empty value are stored as spam and
// not forwarded to webhooks or email.
const HoneypotField = "__fl_hp"

// SubmissionParams holds parameters for creating a submission
type SubmissionParams struct {
	FormID    uint
	DataJSON  string
	UserAgent string
	IsSpam    bool
}

// checkHoneypot returns true when the payload's honeypot field is filled in,
// and removes the field from the payload so it never reaches storage.
func checkHoneypot(payload map[string]any) bool {
	v, ok := payload[HoneypotField]
	if !ok {
		return false
	}
	delete(payload, HoneypotField)
	if s, isStr := v.(string); isStr {
		return strings.TrimSpace(s) != ""
	}
	// Any non-string value still counts as "filled" (bots sometimes
	// submit arrays or other shapes for fields they were never meant
	// to touch).
	return v != nil
}

// CreateSubmission creates a new submission and associated delivery events
func CreateSubmission(logger *slog.Logger, db *gorm.DB, form *Form, payload map[string]any, userAgent string) (*Submission, error) {
	return CreateSubmissionWithFiles(logger, db, form, payload, userAgent, "", nil)
}

// CreateSubmissionWithFiles creates a submission with optional file uploads
func CreateSubmissionWithFiles(logger *slog.Logger, db *gorm.DB, form *Form, payload map[string]any, userAgent string, dataDir string, files []*UploadedFile) (*Submission, error) {
	isSpam := checkHoneypot(payload)

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
		IsSpam:    isSpam,
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		if err := tx.Create(submission).Error; err != nil {
			return err
		}

		// Save files to disk and create records
		if len(files) > 0 && dataDir != "" {
			fileRecords, err := SaveFiles(dataDir, form.ID, submission.ID, files)
			if err != nil {
				return fmt.Errorf("failed to save files: %w", err)
			}
			for _, record := range fileRecords {
				record.SubmissionID = submission.ID
				if err := tx.Create(record).Error; err != nil {
					return err
				}
			}
			submission.Files = fileRecords
		}

		// Spam submissions are stored but never forwarded — the bot sees a
		// success response while the honeypot quietly contains it.
		if !isSpam {
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
		}

		return nil
	}); err != nil {
		// Clean up files on failure
		if len(files) > 0 && dataDir != "" && submission.ID > 0 {
			DeleteSubmissionFiles(dataDir, form.ID, submission.ID)
		}
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
