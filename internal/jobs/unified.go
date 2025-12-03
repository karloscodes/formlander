package jobs

import (
	"time"

	"go.uber.org/zap"

	"formlander/internal/config"
	"formlander/internal/database"
	cartridgeJobs "formlander/internal/pkg/cartridge/jobs"
)

// UnifiedDispatcher runs both webhook and email dispatchers using cartridge's job framework.
type UnifiedDispatcher struct {
	dispatcher *cartridgeJobs.Dispatcher
}

// NewUnifiedDispatcher creates a single dispatcher that handles both webhooks and emails.
func NewUnifiedDispatcher(cfg *config.Config, logger *zap.Logger, db *database.Manager) *UnifiedDispatcher {
	webhooks := NewWebhookDispatcher(cfg)
	emails := NewEmailDispatcher(cfg)

	dispatcher := cartridgeJobs.NewDispatcher(
		logger.Named("unified-dispatcher"),
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
