package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"formlander/internal/config"
	"formlander/internal/database"
	appLogger "formlander/internal/pkg/logger"
)

// Application wires together configuration, logging, database, and HTTP server.
type Application struct {
	Config    *config.Config
	Logger    *slog.Logger
	DBManager *database.Manager
	Server    *Server
}

// ApplicationOptions configure application bootstrapping.
type ApplicationOptions struct {
	Config             *config.Config
	RouteMountFunc     func(*Server)
	CatchAllRedirect   string
	TemplatesFS        fs.FS
	TemplatesDirectory string
	StaticFS           fs.FS
	StaticDirectory    string
}

// NewApplication constructs an application.
func NewApplication(opts ApplicationOptions) (*Application, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Get()
	}

	appLog, err := appLogger.Initialize(cfg)
	if err != nil {
		return nil, fmt.Errorf("server: initialize logger: %w", err)
	}
	slog.SetDefault(appLog)

	dbManager := database.NewManager(cfg, appLog)

	serverCfg := DefaultConfig()
	serverCfg.Config = cfg
	serverCfg.Logger = appLog
	serverCfg.DBManager = dbManager
	serverCfg.TemplatesFS = opts.TemplatesFS
	serverCfg.TemplatesDirectory = opts.TemplatesDirectory
	serverCfg.StaticFS = opts.StaticFS
	serverCfg.StaticDirectory = opts.StaticDirectory

	server, err := NewServer(serverCfg)
	if err != nil {
		return nil, fmt.Errorf("server: create server: %w", err)
	}
	if opts.CatchAllRedirect != "" {
		server.SetCatchAllRedirect(opts.CatchAllRedirect)
	}
	if opts.RouteMountFunc != nil {
		opts.RouteMountFunc(server)
	}

	return &Application{
		Config:    cfg,
		Logger:    appLog,
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
func (a *Application) Run() error {
	return a.RunWithTimeout(10 * time.Second)
}

// RunWithTimeout starts the application with the specified shutdown timeout.
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
		a.Logger.Error("graceful shutdown failed", slog.Any("error", err))
		return err
	}

	a.Logger.Info("shutdown complete")
	return nil
}
