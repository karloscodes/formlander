package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

// LogLevel represents the logging severity.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Environment constants.
const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
	EnvironmentTest        = "test"
)

const defaultDatabaseFilename = "formlander.db"

// Config encapsulates runtime configuration sourced from environment variables.
type Config struct {
	AppName     string `mapstructure:"appname"`
	Environment string `mapstructure:"environment"`
	Port        string `mapstructure:"port"`
	Debug       bool   `mapstructure:"debug"`

	LogLevel         LogLevel `mapstructure:"loglevel"`
	LogsDirectory    string   `mapstructure:"logsdirectory"`
	LogsMaxSizeInMB  int      `mapstructure:"logsmaxsizeinmb"`
	LogsMaxBackups   int      `mapstructure:"logsmaxbackups"`
	LogsMaxAgeInDays int      `mapstructure:"logsmaxageindays"`

	SessionSecret         string `mapstructure:"sessionsecret"`
	SessionTimeoutSeconds int    `mapstructure:"sessiontimeoutseconds"`

	DataDirectory        string `mapstructure:"datadirectory"`
	DatabaseFilename     string `mapstructure:"databasefilename"`
	DatabasePathOverride string `mapstructure:"databasepathoverride"`
	DatabasePath         string `mapstructure:"-"`
	DatabaseMaxOpenConns int    `mapstructure:"databasemaxopenconns"`
	DatabaseMaxIdleConns int    `mapstructure:"databasemaxidleconns"`
	UploadsDirectory     string `mapstructure:"uploadsdirectory"`
	MaxUploadSizeMB      int    `mapstructure:"maxuploadssizemb"`
	MaxInputFields       int    `mapstructure:"maxinputfields"`
	MaxPayloadSizeMB     int    `mapstructure:"maxpayloadsizemb"`
	SubmissionRatePerHour int    `mapstructure:"submissionrateperhour"`

	Webhook WebhookConfig `mapstructure:"webhook"`
}

// WebhookConfig configures outbound webhook delivery.
type WebhookConfig struct {
	SignatureHeader string `mapstructure:"signatureheader"`
	RetryLimit      int    `mapstructure:"retrylimit"`
	BackoffSchedule string `mapstructure:"backoffschedule"`
}

var (
	cfgOnce sync.Once
	cfgInst *Config
)

// Get returns the singleton configuration instance populated from environment variables.
func Get() *Config {
	cfgOnce.Do(func() {
		v := viper.New()
		
		// Set config name and paths for loading .env files
		v.SetConfigName(".env")
		v.SetConfigType("env")
		v.AddConfigPath(".")
		
		// Set defaults
		setDefaults(v)
		
		// Read .env file if it exists (optional)
		_ = v.ReadInConfig()
		
		// Environment variables take precedence
		v.SetEnvPrefix("FORMLANDER")
		
		// Bind all environment variables explicitly
		bindEnvVars(v)
		
		cfgInst = &Config{}
		if err := v.Unmarshal(cfgInst); err != nil {
			log.Fatalf("config: failed to unmarshal configuration: %v", err)
		}

		cfgInst.DatabasePath = cfgInst.resolveDatabasePath()
		cfgInst.ensureDirectories()

		if err := cfgInst.Validate(); err != nil {
			log.Fatalf("config: invalid configuration: %v", err)
		}
	})
	return cfgInst
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("appname", "formlander")
	v.SetDefault("environment", "production")
	v.SetDefault("port", "8080")
	v.SetDefault("debug", false)
	
	v.SetDefault("loglevel", "error")
	v.SetDefault("logsdirectory", "storage/logs")
	v.SetDefault("logsmaxsizeinmb", 20)
	v.SetDefault("logsmaxbackups", 10)
	v.SetDefault("logsmaxageindays", 30)
	
	v.SetDefault("sessiontimeoutseconds", 604800) // 1 week
	
	v.SetDefault("datadirectory", "storage")
	v.SetDefault("databasefilename", "formlander.db")
	v.SetDefault("databasemaxopenconns", 0)
	v.SetDefault("databasemaxidleconns", 0)
	v.SetDefault("uploadsdirectory", "uploads")
	v.SetDefault("maxuploadssizemb", 10)
	v.SetDefault("maxinputfields", 200)
	v.SetDefault("maxpayloadsizemb", 2)
	v.SetDefault("submissionrateperhour", 120)
	
	v.SetDefault("webhook.signatureheader", "X-Formlander-Signature")
	v.SetDefault("webhook.retrylimit", 3)
	v.SetDefault("webhook.backoffschedule", "1,5,15,60")
}

func bindEnvVars(v *viper.Viper) {
	// Bind essential configuration variables
	// All other settings use defaults from setDefaults()
	v.BindEnv("environment", "FORMLANDER_ENV")
	v.BindEnv("port", "FORMLANDER_PORT")
	v.BindEnv("sessionsecret", "FORMLANDER_SESSION_SECRET")
	v.BindEnv("loglevel", "FORMLANDER_LOG_LEVEL")
	v.BindEnv("datadirectory", "FORMLANDER_DATA_DIR")
}

func (c *Config) Validate() error {
	var problems []string

	// In production, REQUIRE session secret
	if c.IsProduction() {
		if c.SessionSecret == "" {
			problems = append(problems, "FORMLANDER_SESSION_SECRET is REQUIRED in production (generate once with: openssl rand -hex 32)")
		}
	} else {
		// Use a fixed secret in non-production (development/test) for session persistence
		if c.SessionSecret == "" {
			c.SessionSecret = "dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4"
			if c.IsDevelopment() {
				log.Println("ℹ️  Using default development secret (set FORMLANDER_SESSION_SECRET for custom value)")
			}
		}
	}

	switch c.Environment {
	case EnvironmentDevelopment, EnvironmentProduction, EnvironmentTest:
	default:
		problems = append(problems, fmt.Sprintf("invalid FORMLANDER_ENV value %q", c.Environment))
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("failed to generate random secret: %v", err)
	}
	return hex.EncodeToString(b)
}

// DatabaseDSN returns the DSN for opening the SQLite database.
func (c *Config) DatabaseDSN() string {
	return c.DatabasePath
}

// IsDevelopment reports whether the application runs in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Environment == EnvironmentDevelopment
}

// IsProduction reports whether the application runs in production mode.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}

// IsTest reports whether the application runs in test mode.
func (c *Config) IsTest() bool {
	return c.Environment == EnvironmentTest
}

// GetPort returns the HTTP server port (implements cartridge.Config interface).
func (c *Config) GetPort() string {
	return c.Port
}

// GetPublicDirectory returns the path to public/static assets (implements cartridge.Config interface).
func (c *Config) GetPublicDirectory() string {
	return "web/static" // Static assets directory
}

// GetAssetsPrefix returns the URL prefix for static assets (implements cartridge.Config interface).
func (c *Config) GetAssetsPrefix() string {
	return "/assets"
}

// GetMaxOpenConns returns configured or environment-specific max open connections.
func (c *Config) GetMaxOpenConns() int {
	if c.DatabaseMaxOpenConns > 0 {
		return c.DatabaseMaxOpenConns
	}
	if c.IsProduction() {
		return 10
	}
	return 1
}

// GetMaxIdleConns returns configured or environment-specific max idle connections.
func (c *Config) GetMaxIdleConns() int {
	if c.DatabaseMaxIdleConns > 0 {
		return c.DatabaseMaxIdleConns
	}
	if c.IsProduction() {
		return 5
	}
	return 1
}

// GetUploadsDirectory resolves the uploads directory relative to the data directory.
func (c *Config) GetUploadsDirectory() string {
	if filepath.IsAbs(c.UploadsDirectory) {
		return c.UploadsDirectory
	}
	return filepath.Join(c.DataDirectory, c.UploadsDirectory)
}

// WebhookBackoff returns the parsed retry schedule for webhook delivery.
func (c *Config) WebhookBackoff() []int {
	if c.Webhook.BackoffSchedule == "" {
		return []int{1, 5, 15, 60}
	}
	parts := strings.Split(c.Webhook.BackoffSchedule, ",")
	backoff := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if val, err := strconv.Atoi(part); err == nil {
			backoff = append(backoff, val)
		}
	}
	if len(backoff) == 0 {
		return []int{1, 5, 15, 60}
	}
	return backoff
}

func (c *Config) ensureDirectories() {
	if err := os.MkdirAll(c.DataDirectory, 0o755); err != nil {
		log.Printf("config: failed to create data directory %q: %v", c.DataDirectory, err)
	}

	if err := os.MkdirAll(c.LogsDirectory, 0o755); err != nil {
		log.Printf("config: failed to create logs directory %q: %v", c.LogsDirectory, err)
	}

	if uploads := c.GetUploadsDirectory(); uploads != "" {
		if err := os.MkdirAll(uploads, 0o755); err != nil {
			log.Printf("config: failed to create uploads directory %q: %v", uploads, err)
		}
	}
}

func (c *Config) resolveDatabasePath() string {
	if c.DatabasePathOverride != "" {
		if filepath.IsAbs(c.DatabasePathOverride) {
			return c.DatabasePathOverride
		}
		return filepath.Join(c.DataDirectory, c.DatabasePathOverride)
	}

	filename := c.DatabaseFilename
	if filename == "" {
		filename = defaultDatabaseFilename
	}

	if strings.EqualFold(filename, defaultDatabaseFilename) {
		filename = addEnvironmentSuffix(filename, c.Environment)
	}

	if filepath.IsAbs(filename) {
		return filename
	}
	return filepath.Join(c.DataDirectory, filename)
}

func addEnvironmentSuffix(filename, environment string) string {
	env := strings.ToLower(strings.TrimSpace(environment))
	if env == "" {
		env = EnvironmentDevelopment
	}

	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	if ext == "" {
		ext = ".db"
	}
	return fmt.Sprintf("%s.%s%s", base, env, ext)
}

// Reset clears the cached configuration; intended for tests.
func Reset() {
	cfgOnce = sync.Once{}
	cfgInst = nil
}
