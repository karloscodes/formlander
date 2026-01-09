# Background Job Pattern

## Overview

Background jobs process webhooks and emails asynchronously with retry logic and state management.

## Location

`internal/jobs/`

## Job Processor Structure

```go
type WebhookDispatcher struct {
    cfg   *config.Config
    http  *http.Client
    retry *RetryStrategy
}

func (d *WebhookDispatcher) ProcessBatch(ctx *cartridge.JobContext) error {
    db := ctx.DB
    now := time.Now().UTC()

    var events []forms.WebhookEvent
    if err := db.
        Preload("Submission").
        Preload("Submission.Form.WebhookDelivery").
        Where("status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
            []string{forms.WebhookStatusPending, forms.WebhookStatusRetrying}, now).
        Order("created_at ASC").
        Limit(10).
        Find(&events).Error; err != nil {
        return err
    }

    for i := range events {
        d.handleEvent(ctx, db, &events[i])
    }
    return nil
}
```

## State Machine

Events follow a state machine:

```
pending -> delivering -> delivered
                     -> retrying -> delivered
                                 -> failed
```

Status constants:

```go
const (
    WebhookStatusPending    = "pending"
    WebhookStatusDelivering = "delivering"
    WebhookStatusDelivered  = "delivered"
    WebhookStatusRetrying   = "retrying"
    WebhookStatusFailed     = "failed"

    DefaultRetryLimit = 3
)
```

## Retry Strategy

```go
type RetryStrategy struct {
    cfg *config.Config
}

func (r *RetryStrategy) NextRetry(attempt int) *time.Time {
    schedule := r.cfg.WebhookBackoff()  // e.g., [60, 300, 900] seconds
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

func (r *RetryStrategy) ShouldRetry(attemptCount int) bool {
    return attemptCount < forms.DefaultRetryLimit
}
```

## Event Updates with Options

Use functional options for flexible updates:

```go
type UpdateOption func(map[string]any)

func WithAttemptCount(count int) UpdateOption {
    return func(values map[string]any) { values["attempt_count"] = count }
}

func WithNextAttempt(next *time.Time) UpdateOption {
    return func(values map[string]any) { values["next_attempt_at"] = next }
}

func (u *EventUpdater) Update(ctx *JobContext, db *gorm.DB, id uint, status string, opts ...UpdateOption) error {
    values := map[string]any{"status": status}
    for _, opt := range opts {
        opt(values)
    }
    return dbtxn.WithRetry(ctx.Logger, db, func(tx *gorm.DB) error {
        return tx.Model(u.model).Where("id = ?", id).Updates(values).Error
    })
}
```

## Key Principles

1. **Batch processing**: Process multiple events per tick
2. **Idempotent**: Safe to retry failed jobs
3. **Backoff**: Exponential delays between retries
4. **State tracking**: Clear status transitions
5. **Retry with dbtxn**: All DB writes use `dbtxn.WithRetry()`
