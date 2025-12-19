package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"log/slog"
)

// RequestLogger emits structured request logs using zap.
func RequestLogger(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		stop := time.Since(start)

		logger.Info("http request",
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", c.Response().StatusCode()),
			slog.Duration("duration", stop),
			slog.String("ip", c.IP()),
		)

		return err
	}
}
