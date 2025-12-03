package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

// ConcurrencyLimiter manages concurrent read and write operations.
type ConcurrencyLimiter struct {
	readSem  *semaphore.Weighted
	writeSem *semaphore.Weighted
	timeout  time.Duration
	logger   *zap.Logger
}

// NewConcurrencyLimiter constructs a limiter with the provided thresholds.
func NewConcurrencyLimiter(readLimit, writeLimit int64, timeout time.Duration, logger *zap.Logger) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		readSem:  semaphore.NewWeighted(readLimit),
		writeSem: semaphore.NewWeighted(writeLimit),
		timeout:  timeout,
		logger:   logger,
	}
}

// WriteConcurrencyLimitMiddleware limits concurrent write operations to protect database integrity.
// For SQLite with WAL mode, this prevents write contention while allowing reasonable concurrency.
func WriteConcurrencyLimitMiddleware(limiter *ConcurrencyLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip for OPTIONS (CORS preflight)
		if c.Method() == fiber.MethodOptions {
			return c.Next()
		}

		// Check if parent context is already canceled
		if err := c.Context().Err(); err != nil {
			limiter.logger.Debug("Request context already canceled",
				zap.String("path", c.Path()),
				zap.Error(err),
			)
			return c.Status(fiber.StatusRequestTimeout).JSON(fiber.Map{
				"error":   "Request Timeout",
				"message": "Request was canceled",
			})
		}

		// Create timeout context from request context (not background context)
		ctx, cancel := context.WithTimeout(c.Context(), limiter.timeout)
		defer cancel()

		start := time.Now()
		if err := limiter.AcquireWrite(ctx); err != nil {
			waitTime := time.Since(start)
			limiter.logger.Warn("Write concurrency limit reached",
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()),
				zap.String("method", c.Method()),
				zap.Duration("wait_time", waitTime),
				zap.Error(err),
			)

			// Return appropriate error based on context
			if ctx.Err() == context.DeadlineExceeded {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"error":       "Service Unavailable",
					"message":     "Server is at capacity processing writes, please retry",
					"retry_after": "1", // Suggest retry after 1 second
				})
			}

			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":   "Service Unavailable",
				"message": "Write operation could not be queued",
			})
		}
		defer limiter.ReleaseWrite()

		acquireTime := time.Since(start)
		if acquireTime > 100*time.Millisecond {
			limiter.logger.Info("Write operation queued (high load detected)",
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()),
				zap.Duration("queue_time", acquireTime),
			)
		}

		return c.Next()
	}
}

// AcquireRead acquires a read semaphore
func (cl *ConcurrencyLimiter) AcquireRead(ctx context.Context) error {
	return cl.readSem.Acquire(ctx, 1)
}

// AcquireWrite acquires a write semaphore
func (cl *ConcurrencyLimiter) AcquireWrite(ctx context.Context) error {
	return cl.writeSem.Acquire(ctx, 1)
}

// ReleaseRead releases a read semaphore
func (cl *ConcurrencyLimiter) ReleaseRead() {
	cl.readSem.Release(1)
}

// ReleaseWrite releases a write semaphore
func (cl *ConcurrencyLimiter) ReleaseWrite() {
	cl.writeSem.Release(1)
}
