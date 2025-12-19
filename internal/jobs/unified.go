package jobs

import (
	"time"

	"log/slog"

	"formlander/internal/config"
	"formlander/internal/database"
)

// UnifiedDispatcher runs both webhook and email dispatchers using the job framework.
type UnifiedDispatcher struct {
	dispatcher *Dispatcher
}

// NewUnifiedDispatcher creates a single dispatcher that handles both webhooks and emails.
func NewUnifiedDispatcher(cfg *config.Config, logger *slog.Logger, db *database.Manager) *UnifiedDispatcher {
	webhooks := NewWebhookDispatcher(cfg)
	emails := NewEmailDispatcher(cfg)

	dispatcher := NewDispatcher(
		logger.With(slog.String("component", "unified-dispatcher")),
		db,
		2*time.Minute,
		webhooks,
		emails,
	)

	return &UnifiedDispatcher{
		dispatcher: dispatcher,
	}
}

// Start begins the unified background processing loop.
func (d *UnifiedDispatcher) Start() error {
	return d.dispatcher.Start()
}

// Stop terminates the dispatcher and waits for completion.
func (d *UnifiedDispatcher) Stop() {
	d.dispatcher.Stop()
}
