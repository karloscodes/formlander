package jobs

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"formlander/internal/database"
)

// JobContext provides job-scoped access to application dependencies.
type JobContext struct {
	context.Context
	Logger *zap.Logger
	DB     *gorm.DB
}

// Processor defines the interface for processing a batch of work.
type Processor interface {
	ProcessBatch(ctx *JobContext) error
}

// Dispatcher runs processors periodically in a background loop.
type Dispatcher struct {
	logger     *zap.Logger
	dbManager  *database.Manager
	processors []Processor
	interval   time.Duration
	mu         sync.Mutex
	running    bool
	stop       chan struct{}
	wg         sync.WaitGroup
}

// NewDispatcher creates a new background job dispatcher.
func NewDispatcher(logger *zap.Logger, dbManager *database.Manager, interval time.Duration, processors ...Processor) *Dispatcher {
	return &Dispatcher{
		logger:     logger.Named("dispatcher"),
		dbManager:  dbManager,
		processors: processors,
		interval:   interval,
	}
}

// Start begins the background processing loop.
func (d *Dispatcher) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return nil
	}

	d.stop = make(chan struct{})
	d.running = true
	d.wg.Add(1)
	go d.loop()
	return nil
}

// Stop terminates the dispatcher and waits for completion.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	close(d.stop)
	d.running = false
	d.mu.Unlock()
	d.wg.Wait()
}

func (d *Dispatcher) loop() {
	defer d.wg.Done()

	d.logger.Info("dispatcher started", zap.Int("processors", len(d.processors)))
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Run immediately on startup
	d.processBatch()

	for {
		select {
		case <-ticker.C:
			d.processBatch()
		case <-d.stop:
			d.logger.Info("dispatcher stopped")
			return
		}
	}
}

func (d *Dispatcher) processBatch() {
	db, err := d.dbManager.Connect()
	if err != nil {
		d.logger.Error("failed to connect to database", zap.Error(err))
		return
	}

	ctx := &JobContext{
		Context: context.Background(),
		Logger:  d.logger,
		DB:      db,
	}

	for _, processor := range d.processors {
		if err := processor.ProcessBatch(ctx); err != nil {
			d.logger.Error("processor failed", zap.Error(err))
		}
	}
}

// IsRunning returns whether the dispatcher is currently running.
func (d *Dispatcher) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}
