package cartridge

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"formlander/internal/config"
	"formlander/internal/database"
	appLogger "formlander/internal/pkg/logger"
)

// Application wires together configuration, logging, database, and HTTP server.
// It manages the complete lifecycle of a cartridge web application.
type Application struct {
	Config    *config.Config    // Runtime configuration
	Logger    *zap.Logger       // Global application logger
	DBManager *database.Manager // Database connection pool manager
	Server    *Server           // HTTP server with routes
}

// ApplicationOptions configure application bootstrapping.
type ApplicationOptions struct {
	Config             *config.Config
	RouteMountFunc     func(*Server)
	CatchAllRedirect   string
	TemplatesFS        fs.FS  // Embedded filesystem for templates
	TemplatesDirectory string // Custom template directory (development mode)
	StaticFS           fs.FS  // Embedded filesystem for static assets
	StaticDirectory    string // Custom static directory (development mode)
}

// NewApplication constructs a cartridge application.
func NewApplication(opts ApplicationOptions) (*Application, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Get()
	}

	zapLogger, err := appLogger.Initialize(cfg)
	if err != nil {
		return nil, fmt.Errorf("cartridge: initialize logger: %w", err)
	}
	zap.ReplaceGlobals(zapLogger)

	dbManager := database.NewManager(cfg, zapLogger)

	serverCfg := DefaultConfig()
	serverCfg.Config = cfg
	serverCfg.Logger = zapLogger
	serverCfg.DBManager = dbManager
	serverCfg.TemplatesFS = opts.TemplatesFS
	serverCfg.TemplatesDirectory = opts.TemplatesDirectory
	serverCfg.StaticFS = opts.StaticFS
	serverCfg.StaticDirectory = opts.StaticDirectory

	server, err := NewServer(serverCfg)
	if err != nil {
		return nil, fmt.Errorf("cartridge: create server: %w", err)
	}
	if opts.CatchAllRedirect != "" {
		server.SetCatchAllRedirect(opts.CatchAllRedirect)
	}
	if opts.RouteMountFunc != nil {
		opts.RouteMountFunc(server)
	}

	return &Application{
		Config:    cfg,
		Logger:    zapLogger,
		DBManager: dbManager,
		Server:    server,
	}, nil
}

// Start launches the HTTP server.
func (a *Application) Start() error {
	return a.Server.Start()
}

// StartAsync launches the HTTP server asynchronously.
func (a *Application) StartAsync() error {
	return a.Server.StartAsync()
}

// Shutdown gracefully stops the server and closes resources.
func (a *Application) Shutdown(ctx context.Context) error {
	if err := a.Server.Shutdown(ctx); err != nil {
		return err
	}
	return a.DBManager.Close()
}

// Run starts the application and waits for termination signals.
// It handles graceful shutdown with a default timeout of 10 seconds.
func (a *Application) Run() error {
	return a.RunWithTimeout(10 * time.Second)
}

// RunWithTimeout starts the application and waits for termination signals.
// It handles graceful shutdown with the specified timeout.
func (a *Application) RunWithTimeout(timeout time.Duration) error {
	if err := a.Start(); err != nil {
		return err
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	a.Logger.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		a.Logger.Error("graceful shutdown failed", zap.Error(err))
		return err
	}

	a.Logger.Info("shutdown complete")
	return nil
}
