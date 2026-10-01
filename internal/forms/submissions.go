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
	switch value := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(value) != ""
	case bool:
		// A JS client may send the untouched field as false.
		return value
	case float64:
		return value != 0
	default:
		// Arrays and objects count as filled: bots send odd shapes for
		// fields they were never meant to touch.
		return true
	}
}

// CreateSubmission creates a new submission and associated delivery events
func CreateSubmission(logger *slog.Logger, db *gorm.DB, form *Form, payload map[string]any, userAgent string) (*Submission, error) {
	return CreateSubmissionWithFiles(logger, db, form, payload, userAgent, "", nil)
}

// CreateSubmissionWithFiles creates a submission with optional file uploads
func CreateSubmissionWithFiles(logger *slog.Logger, db *gorm.DB, form *Form, payload map[string]any, userAgent string, dataDir string, files []*UploadedFile) (*Submission, error) {
	isSpam := checkHoneypot(payload)
	if isSpam {
		// Operator visibility into honeypot activity. Info-level so it
		// can be filtered out at scale but is on by default.
		logger.Info("honeypot triggered",
			slog.Uint64("form_id", uint64(form.ID)),
			slog.String("form_slug", form.Slug),
		)
	}

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

		// Save files to disk and create records.
		// Spam submissions don't save files: the bot got its 2xx, but we
		// don't want to give it a free disk-fill vector for forms that
		// accept uploads.
		if !isSpam && len(files) > 0 && dataDir != "" {
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
			if form.WebhookDelivery.Delivers() {
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

// DeleteSubmissions removes submissions for good: the rows, their delivery
// attempts, and their uploaded files. It returns how many it removed. IDs
// that do not exist are skipped.
func DeleteSubmissions(logger *slog.Logger, db *gorm.DB, dataDir string, ids []uint) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	var submissions []Submission
	if err := db.Select("id", "form_id").Where("id IN ?", ids).Find(&submissions).Error; err != nil {
		return 0, err
	}
	if len(submissions) == 0 {
		return 0, nil
	}
	found := make([]uint, len(submissions))
	for i, submission := range submissions {
		found[i] = submission.ID
	}

	// The children go first: SQLite removes them itself only when foreign
	// keys are on for the connection.
	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		for _, child := range []any{&WebhookEvent{}, &EmailEvent{}, &SubmissionFile{}} {
			if err := tx.Where("submission_id IN ?", found).Delete(child).Error; err != nil {
				return err
			}
		}
		return tx.Where("id IN ?", found).Delete(&Submission{}).Error
	}); err != nil {
		logger.Error("delete submissions failed", slog.Any("error", err))
		return 0, err
	}

	// The rows are gone, so a file that stays behind is only wasted space.
	if dataDir != "" {
		for _, submission := range submissions {
			if err := DeleteSubmissionFiles(dataDir, submission.FormID, submission.ID); err != nil {
				logger.Warn("delete submission files failed", slog.Uint64("submission_id", uint64(submission.ID)), slog.Any("error", err))
			}
		}
	}

	return len(submissions), nil
}

// DeleteSpam removes every submission marked as spam, of one form or, with
// formID 0, of all forms. It returns how many it removed.
func DeleteSpam(logger *slog.Logger, db *gorm.DB, dataDir string, formID uint) (int, error) {
	query := db.Model(&Submission{}).Where("is_spam = ?", true)
	if formID != 0 {
		query = query.Where("form_id = ?", formID)
	}
	var ids []uint
	if err := query.Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	return DeleteSubmissions(logger, db, dataDir, ids)
}
