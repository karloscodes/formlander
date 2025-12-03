package jobs

import (
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/pkg/cartridge/jobs"
	"formlander/internal/pkg/dbtxn"
)

// RetryStrategy handles retry logic for background jobs.
type RetryStrategy struct {
	cfg *config.Config
}

// NewRetryStrategy creates a retry strategy.
func NewRetryStrategy(cfg *config.Config) *RetryStrategy {
	return &RetryStrategy{cfg: cfg}
}

// NextRetry calculates the next retry time based on attempt count.
func (r *RetryStrategy) NextRetry(attempt int) *time.Time {
	schedule := r.cfg.WebhookBackoff()
	if len(schedule) == 0 {
		return nil
	}
	idx := attempt - 1
	if idx >= len(schedule) {
		idx = len(schedule) - 1
	}
	next := time.Now().UTC().Add(time.Duration(schedule[idx]) * time.Second)
	return &next
}

// ShouldRetry returns true if the event should be retried.
func (r *RetryStrategy) ShouldRetry(attemptCount int) bool {
	return attemptCount < forms.DefaultRetryLimit
}

// TruncateError truncates error messages to a reasonable length.
func TruncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.TrimSpace(msg)
	if len(msg) <= 500 {
		return msg
	}
	return msg[:500]
}

// UpdateOption is a functional option for updating event fields.
type UpdateOption func(map[string]any)

// WithAttemptCount sets the attempt count.
func WithAttemptCount(count int) UpdateOption {
	return func(values map[string]any) { values["attempt_count"] = count }
}

// WithNextAttempt sets the next attempt time.
func WithNextAttempt(next *time.Time) UpdateOption {
	return func(values map[string]any) { values["next_attempt_at"] = next }
}

// EventUpdater provides a generic way to update webhook/email events.
type EventUpdater struct {
	model interface{}
}

// NewEventUpdater creates an event updater for the given model type.
func NewEventUpdater(model interface{}) *EventUpdater {
	return &EventUpdater{model: model}
}

// Update performs a transactional update of the event.
func (u *EventUpdater) Update(ctx *jobs.JobContext, db *gorm.DB, id uint, status string, attemptTime time.Time, message string, opts ...UpdateOption) error {
	values := map[string]any{
		"status":           status,
		"last_attempt_at":  attemptTime.UTC(),
		"last_attempt_err": message,
	}
	for _, opt := range opts {
		opt(values)
	}

	return dbtxn.WithRetry(ctx.Logger, db, func(tx *gorm.DB) error {
		return tx.Model(u.model).
			Where("id = ?", id).
			Updates(values).Error
	})
}

// MarkAsRetry marks an event for retry with backoff.
func MarkWebhookAsRetry(ctx *jobs.JobContext, db *gorm.DB, event *forms.WebhookEvent, strategy *RetryStrategy, err error) {
	attemptCount := event.AttemptCount + 1
	status := forms.WebhookStatusRetrying
	var nextAttempt *time.Time
	message := TruncateError(err)

	if !strategy.ShouldRetry(attemptCount) {
		status = forms.WebhookStatusFailed
	} else {
		nextAttempt = strategy.NextRetry(attemptCount)
	}

	updater := NewEventUpdater(&forms.WebhookEvent{})
	if err := updater.Update(ctx, db, event.ID, status, time.Now(), message, WithAttemptCount(attemptCount), WithNextAttempt(nextAttempt)); err != nil {
		ctx.Logger.Error("update retry webhook", zap.Uint("id", event.ID), zap.Error(err))
		return
	}

	// Update in-memory event
	event.Status = status
	event.AttemptCount = attemptCount
	last := time.Now().UTC()
	event.LastAttemptAt = &last
	event.LastAttemptErr = message
	event.NextAttemptAt = nextAttempt
}

// MarkWebhookAsFinal marks an event as final (delivered or failed).
func MarkWebhookAsFinal(ctx *jobs.JobContext, db *gorm.DB, event *forms.WebhookEvent, status, message string) {
	updater := NewEventUpdater(&forms.WebhookEvent{})
	if err := updater.Update(ctx, db, event.ID, status, time.Now(), message, WithNextAttempt(nil)); err != nil {
		ctx.Logger.Error("finalize webhook", zap.Uint("id", event.ID), zap.Error(err))
		return
	}

	// Update in-memory event
	event.Status = status
	last := time.Now().UTC()
	event.LastAttemptAt = &last
	event.LastAttemptErr = message
	event.NextAttemptAt = nil
}

// MarkEmailAsRetry marks an email event for retry with backoff.
func MarkEmailAsRetry(ctx *jobs.JobContext, db *gorm.DB, event *forms.EmailEvent, strategy *RetryStrategy, err error) {
	attemptCount := event.AttemptCount + 1
	status := forms.WebhookStatusRetrying
	var nextAttempt *time.Time
	message := TruncateError(err)

	if !strategy.ShouldRetry(attemptCount) {
		status = forms.WebhookStatusFailed
	} else {
		nextAttempt = strategy.NextRetry(attemptCount)
	}

	updater := NewEventUpdater(&forms.EmailEvent{})
	if err := updater.Update(ctx, db, event.ID, status, time.Now(), message, WithAttemptCount(attemptCount), WithNextAttempt(nextAttempt)); err != nil {
		ctx.Logger.Error("update email retry", zap.Uint("id", event.ID), zap.Error(err))
		return
	}

	// Update in-memory event
	event.Status = status
	event.AttemptCount = attemptCount
	last := time.Now().UTC()
	event.LastAttemptAt = &last
	event.LastAttemptErr = message
	event.NextAttemptAt = nextAttempt
}

// MarkEmailAsFinal marks an email event as final (delivered or failed).
func MarkEmailAsFinal(ctx *jobs.JobContext, db *gorm.DB, event *forms.EmailEvent, status, message string) {
	updater := NewEventUpdater(&forms.EmailEvent{})
	if err := updater.Update(ctx, db, event.ID, status, time.Now(), message, WithNextAttempt(nil)); err != nil {
		ctx.Logger.Error("finalize email event", zap.Uint("id", event.ID), zap.Error(err))
		return
	}

	// Update in-memory event
	event.Status = status
	last := time.Now().UTC()
	event.LastAttemptAt = &last
	event.LastAttemptErr = message
	event.NextAttemptAt = nil
}
