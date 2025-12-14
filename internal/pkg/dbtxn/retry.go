package dbtxn

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"log/slog"
	"gorm.io/gorm"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// WithRetry executes a write transaction with retry logic for SQLite busy errors.
// Given the current database configuration (busy_timeout=5000, WAL mode, _txlock=immediate),
// retries should be rare but provide an additional safety layer for high-concurrency scenarios.
func WithRetry(logger *slog.Logger, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	const (
		maxRetries = 10
		baseDelay  = 100 * time.Millisecond
		maxDelay   = 5 * time.Second
	)

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > maxDelay {
				delay = maxDelay
			}
			delay += time.Duration(rand.Float64() * 0.2 * float64(delay))
			logger.Info("retrying transaction", slog.Int("attempt", attempt+1), slog.Duration("delay", delay), slog.Any("error", err))
			time.Sleep(delay)
		}

		tx := db.Session(&gorm.Session{
			SkipDefaultTransaction: true,
		}).Begin()
		if tx.Error != nil {
			return fmt.Errorf("models: begin transaction: %w", tx.Error)
		}

		err = fn(tx)
		if err != nil {
			tx.Rollback()
			if isBusyError(err) {
				continue
			}
			return err
		}

		if err = tx.Commit().Error; err != nil {
			if isBusyError(err) {
				tx.Rollback()
				continue
			}
			return fmt.Errorf("models: commit transaction: %w", err)
		}

		return nil
	}

	return fmt.Errorf("models: transaction failed after %d attempts: %w", maxRetries, err)
}

func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database is busy") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "SQL statements in progress")
}
