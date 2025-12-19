package cartridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/template/html/v2"
	"log/slog"

	"formlander/internal/config"
	"formlander/internal/database"
	cartridgeMiddleware "formlander/internal/middleware"
)

// Build info set at compile time via ldflags
var (
	buildCommit = "dev"
)

// Config configures the cartridge server.
// Note: This struct holds dependencies (Logger, Config, DBManager) that are
// injected into each request's Context via wrapHandler.
type Config struct {
	Config    *config.Config    // Runtime configuration
	Logger    *slog.Logger       // Application logger
	DBManager *database.Manager // Database connection pool

	ErrorHandler fiber.ErrorHandler

	EnableTemplates    bool
	TemplatesDirectory string
	TemplatesFS        fs.FS // Embedded filesystem for templates

	EnableStaticAssets bool
	StaticDirectory    string
	StaticPrefix       string
	StaticFS           fs.FS // Embedded filesystem for static assets

	EnableRequestLogger bool
	EnableRequestID     bool
	EnableRecover       bool
	EnableHelmet        bool
	EnableCompress      bool
	RequestTimeout      time.Duration

	MaxConcurrentReads  int
	MaxConcurrentWrites int
	ConcurrencyTimeout  time.Duration
}

// DefaultConfig returns sensible defaults for the server configuration.
func DefaultConfig() *Config {
	return &Config{
		EnableTemplates:     true,
		EnableStaticAssets:  true,
		EnableRequestLogger: true,
		EnableRequestID:     true,
		EnableRecover:       true,
		EnableHelmet:        true,
		EnableCompress:      true,
		RequestTimeout:      30 * time.Second,
		StaticPrefix:        "/assets",
		MaxConcurrentReads:  128,
		MaxConcurrentWrites: 8,
		ConcurrencyTimeout:  5 * time.Second,
	}
}

// RouteConfig customises middleware for a route.
type RouteConfig struct {
	EnableCORS         bool
	CORSConfig         *cors.Config
	WriteConcurrency   bool
	EnableSecFetchSite *bool // CSRF protection, default true. Set to false for public/cross-origin routes.
	// CustomMiddleware are standard fiber handlers (backward compatibility).
	// These run before the route handler and receive fiber.Ctx directly.
	CustomMiddleware []fiber.Handler
}

// Bool returns a pointer to a bool value.
func Bool(v bool) *bool { return &v }

// Server wraps a fiber.App with cartridge defaults.
type Server struct {
	app      *fiber.App
	cfg      *Config
	limiter  *cartridgeMiddleware.ConcurrencyLimiter
	catchAll string
}

// NewServer creates a cartridge server using the provided configuration.
func NewServer(cfg *Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cartridge: config is required")
	}
	if cfg.Config == nil {
		return nil, fmt.Errorf("cartridge: runtime config is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("cartridge: logger is required")
	}
	if cfg.DBManager == nil {
		return nil, fmt.Errorf("cartridge: database manager is required")
	}

	fiberCfg := fiber.Config{
		DisableDefaultDate:    true,
		DisableStartupMessage: true,
		ReadTimeout:           cfg.RequestTimeout,
		WriteTimeout:          cfg.RequestTimeout,
	}

	if cfg.EnableTemplates {
		var engine *html.Engine

		if cfg.TemplatesFS != nil {
			// Use embedded filesystem - convert io/fs.FS to http.FileSystem
			engine = html.NewFileSystem(http.FS(cfg.TemplatesFS), ".html")
		} else {
			// Use directory (development mode)
			dir := cfg.TemplatesDirectory
			if dir == "" {
				dir = "web/templates"
			}
			engine = html.New(dir, ".html")
		}

		engine.AddFunc("render", func(name string, data any) (template.HTML, error) {
			if !engine.Loaded {
				if err := engine.Load(); err != nil {
					return "", err
				}
			}
			tpl := engine.Templates.Lookup(name)
			if tpl == nil {
				return "", fmt.Errorf("template %q not found", name)
			}
			var buf bytes.Buffer
			if err := tpl.Execute(&buf, data); err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		})
		engine.AddFunc("safeHTML", func(s string) template.HTML {
			return template.HTML(s)
		})
		engine.Debug(cfg.Config.IsDevelopment())
		if cfg.Config.IsDevelopment() && cfg.TemplatesFS == nil {
			engine.Reload(true)
		}
		engine.AddFunc("truncateJSON", truncateJSON)
		engine.AddFunc("assetVersion", func() string {
			if buildCommit == "dev" {
				return time.Now().Format("20060102150405")
			}
			if len(buildCommit) > 8 {
				return buildCommit[:8]
			}
			return buildCommit
		})
		fiberCfg.Views = engine
	}

	if cfg.ErrorHandler != nil {
		fiberCfg.ErrorHandler = cfg.ErrorHandler
	} else {
		fiberCfg.ErrorHandler = defaultErrorHandler(cfg.Logger, cfg.Config)
	}

	app := fiber.New(fiberCfg)

	server := &Server{
		app:     app,
		cfg:     cfg,
		limiter: cartridgeMiddleware.NewConcurrencyLimiter(int64(cfg.MaxConcurrentReads), int64(cfg.MaxConcurrentWrites), cfg.ConcurrencyTimeout, cfg.Logger),
	}

	server.setupGlobalMiddleware()
	server.setupStaticAssets()

	return server, nil
}

func (s *Server) setupGlobalMiddleware() {
	if s.cfg.EnableRequestID {
		s.app.Use(requestid.New())
	}

	if s.cfg.EnableRecover {
		s.app.Use(fiberrecover.New())
	}

	if s.cfg.EnableHelmet {
		s.app.Use(helmet.New(helmet.Config{
			// Disable CSP - it's too strict by default and blocks inline styles/scripts.
			// If you need CSP, configure it explicitly for your assets.
			ContentSecurityPolicy: "",
		}))
	}

	if s.cfg.EnableCompress {
		s.app.Use(compress.New(compress.Config{
			Level: compress.LevelDefault,
		}))
	}

	// Sec-Fetch-Site CSRF protection (enabled globally, can be disabled per-route)
	s.app.Use(cartridgeMiddleware.SecFetchSiteMiddleware(cartridgeMiddleware.SecFetchSiteConfig{
		Next: func(c *fiber.Ctx) bool {
			if skip, ok := c.Locals("skip_sec_fetch_site").(bool); ok && skip {
				return true
			}
			return false
		},
	}))

	if s.cfg.EnableRequestLogger {
		s.app.Use(cartridgeMiddleware.RequestLogger(s.cfg.Logger))
	}
}

func (s *Server) setupStaticAssets() {
	if !s.cfg.EnableStaticAssets {
		return
	}

	prefix := s.cfg.StaticPrefix
	if prefix == "" {
		prefix = "/assets"
	}

	staticConfig := fiber.Static{
		Compress:      true,
		ByteRange:     true,
		Browse:        false,
		CacheDuration: 24 * time.Hour,
	}

	if s.cfg.StaticFS != nil {
		// Use embedded filesystem
		s.app.Use(prefix, filesystem.New(filesystem.Config{
			Root:       http.FS(s.cfg.StaticFS),
			Browse:     false,
			MaxAge:     int((24 * time.Hour).Seconds()),
			PathPrefix: "",
		}))
	} else {
		// Use directory (development mode)
		dir := s.cfg.StaticDirectory
		if dir == "" {
			dir = "web/static"
		}
		s.app.Static(prefix, dir, staticConfig)
	}
}

// SetCatchAllRedirect configures a fallback redirect path for unmatched routes.
func (s *Server) SetCatchAllRedirect(path string) {
	s.catchAll = path
}

// Get exposes fiber.App.Get with cartridge route configuration.
func (s *Server) Get(path string, handler func(*Context) error, cfg ...*RouteConfig) {
	s.registerRoute(fiber.MethodGet, path, s.wrapHandler(handler), cfg...)
}

// Post exposes fiber.App.Post with cartridge route configuration.
func (s *Server) Post(path string, handler func(*Context) error, cfg ...*RouteConfig) {
	s.registerRoute(fiber.MethodPost, path, s.wrapHandler(handler), cfg...)
}

// Put exposes fiber.App.Put with cartridge route configuration.
func (s *Server) Put(path string, handler func(*Context) error, cfg ...*RouteConfig) {
	s.registerRoute(fiber.MethodPut, path, s.wrapHandler(handler), cfg...)
}

// Delete exposes fiber.App.Delete with cartridge route configuration.
func (s *Server) Delete(path string, handler func(*Context) error, cfg ...*RouteConfig) {
	s.registerRoute(fiber.MethodDelete, path, s.wrapHandler(handler), cfg...)
}

// Options exposes fiber.App.Options with cartridge route configuration.
func (s *Server) Options(path string, handler func(*Context) error, cfg ...*RouteConfig) {
	s.registerRoute(fiber.MethodOptions, path, s.wrapHandler(handler), cfg...)
}

// wrapHandler converts a cartridge handler to a fiber handler.
func (s *Server) wrapHandler(handler func(*Context) error) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := &Context{
			Ctx:       c,
			Logger:    s.cfg.Logger,
			Config:    s.cfg.Config,
			DBManager: s.cfg.DBManager,
		}
		return handler(ctx)
	}
}

func (s *Server) registerRoute(method, path string, handler fiber.Handler, cfg ...*RouteConfig) {
	var routeCfg *RouteConfig
	if len(cfg) > 0 {
		routeCfg = cfg[0]
	}

	capacity := 1
	if routeCfg != nil {
		capacity += len(routeCfg.CustomMiddleware)
	}
	handlers := make([]fiber.Handler, 0, capacity+1) // +1 for context injector

	// Always inject cartridge context first so custom middleware can access it
	contextInjector := func(c *fiber.Ctx) error {
		ctx := &Context{
			Ctx:       c,
			Logger:    s.cfg.Logger,
			Config:    s.cfg.Config,
			DBManager: s.cfg.DBManager,
		}
		c.Locals("cartridge_ctx", ctx)
		return c.Next()
	}
	handlers = append(handlers, contextInjector)

	if routeCfg != nil {
		// Skip SecFetchSite if explicitly set to false
		if routeCfg.EnableSecFetchSite != nil && !*routeCfg.EnableSecFetchSite {
			handlers = append(handlers, func(c *fiber.Ctx) error {
				c.Locals("skip_sec_fetch_site", true)
				return c.Next()
			})
		}

		if routeCfg.EnableCORS {
			corsCfg := routeCfg.CORSConfig
			if corsCfg == nil {
				corsCfg = &cors.Config{
					AllowOrigins: "*",
					AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
					AllowHeaders: "Origin, Content-Type, Accept, Authorization",
				}
			}
			handlers = append(handlers, cors.New(*corsCfg))
		}

		if routeCfg.WriteConcurrency {
			handlers = append(handlers, cartridgeMiddleware.WriteConcurrencyLimitMiddleware(s.limiter))
		}

		if len(routeCfg.CustomMiddleware) > 0 {
			handlers = append(handlers, routeCfg.CustomMiddleware...)
		}
	}

	handlers = append(handlers, handler)
	s.app.Add(method, path, handlers...)
}

// App exposes the underlying Fiber application for testing.
func (s *Server) App() *fiber.App {
	return s.app
}

// Start listens on the configured port.
func (s *Server) Start() error {
	if s.catchAll != "" {
		s.app.All("*", func(c *fiber.Ctx) error {
			return c.Redirect(s.catchAll, fiber.StatusTemporaryRedirect)
		})
	}
	s.cfg.Logger.Info("starting http server", slog.String("addr", ":"+s.cfg.Config.Port))
	return s.app.Listen(":" + s.cfg.Config.Port)
}

// StartAsync starts the server in a new goroutine.
func (s *Server) StartAsync() error {
	go func() {
		if err := s.Start(); err != nil {
			s.cfg.Logger.Error("fiber listen failed", slog.Any("error", err))
		}
	}()
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- s.app.Shutdown()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func defaultErrorHandler(log *slog.Logger, cfg *config.Config) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		log.Error("request failed",
			slog.Any("error", err),
			slog.String("path", c.Path()),
			slog.String("method", c.Method()),
			slog.Int("status", code),
		)

		// JSON error response for API requests
		if c.Accepts(fiber.MIMEApplicationJSON) == fiber.MIMEApplicationJSON {
			return c.Status(code).JSON(fiber.Map{
				"error":   "internal_server_error",
				"message": err.Error(),
			})
		}

		// HTML error page for browser requests
		if code == fiber.StatusInternalServerError {
			return c.Status(code).Render("layouts/base", fiber.Map{
				"Title":             "500 - Internal Server Error",
				"ContentView":       "errors/500/content",
				"DevMode":           cfg.IsDevelopment(),
				"ErrorMessage":      err.Error(),
				"HideHeaderActions": true,
			}, "")
		}

		// Fallback for other error codes
		return c.Status(code).SendString(fmt.Sprintf("Error: %d - %s", code, err.Error()))
	}
}

func truncateJSON(raw string) string {
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if canonical, err := json.Marshal(payload); err == nil {
			raw = string(canonical)
		}
	}
	const limit = 80
	if len(raw) <= limit {
		return raw
	}
	return raw[:limit] + "…"
}
