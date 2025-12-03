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

	"github.com/kelseyhightower/envconfig"
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
	AppName     string `envconfig:"FORMLANDER_APP_NAME" default:"formlander"`
	Environment string `envconfig:"FORMLANDER_ENV" default:"development"`
	Port        string `envconfig:"FORMLANDER_PORT" default:"8080"`
	Debug       bool   `envconfig:"FORMLANDER_DEBUG" default:"false"`

	LogLevel         LogLevel `envconfig:"FORMLANDER_LOG_LEVEL" default:"info"`
	LogsDirectory    string   `envconfig:"FORMLANDER_LOGS_DIR" default:"storage/logs"`
	LogsMaxSizeInMB  int      `envconfig:"FORMLANDER_LOGS_MAX_SIZE_MB" default:"20"`
	LogsMaxBackups   int      `envconfig:"FORMLANDER_LOGS_MAX_BACKUPS" default:"10"`
	LogsMaxAgeInDays int      `envconfig:"FORMLANDER_LOGS_MAX_AGE_DAYS" default:"30"`

	// Security: HMAC secret for signing session cookies. Auto-generated if not provided.
	SessionSecret string `envconfig:"FORMLANDER_SESSION_SECRET"`
	// Security: Salt for hashing IP addresses before storage (privacy). Auto-generated if not provided.
	AnonSalt              string `envconfig:"FORMLANDER_ANON_SALT"`
	SessionTimeoutSeconds int    `envconfig:"FORMLANDER_SESSION_TIMEOUT_SECONDS" default:"604800"` // 1 week

	DataDirectory         string `envconfig:"FORMLANDER_DATA_DIR" default:"storage"`
	DatabaseFilename      string `envconfig:"FORMLANDER_DATABASE_FILENAME" default:"formlander.db"`
	DatabasePathOverride  string `envconfig:"FORMLANDER_DATABASE_PATH"`
	DatabasePath          string
	DatabaseMaxOpenConns  int    `envconfig:"FORMLANDER_DB_MAX_OPEN_CONNS" default:"0"`
	DatabaseMaxIdleConns  int    `envconfig:"FORMLANDER_DB_MAX_IDLE_CONNS" default:"0"`
	UploadsDirectory      string `envconfig:"FORMLANDER_UPLOADS_DIR" default:"uploads"`
	MaxUploadSizeMB       int    `envconfig:"FORMLANDER_MAX_UPLOAD_MB" default:"10"`
	MaxInputFields        int    `envconfig:"FORMLANDER_MAX_FIELDS" default:"200"`
	MaxPayloadSizeMB      int    `envconfig:"FORMLANDER_MAX_PAYLOAD_MB" default:"2"`
	SubmissionRatePerHour int    `envconfig:"FORMLANDER_SUBMISSION_RATE_PER_HOUR" default:"120"`

	// Note: Rate limiting is hardcoded to 30 requests per 60 seconds per IP

	Webhook WebhookConfig
}

// WebhookConfig configures outbound webhook delivery.
type WebhookConfig struct {
	SignatureHeader string `envconfig:"FORMLANDER_WEBHOOK_SIGNATURE_HEADER" default:"X-Formlander-Signature"`
	RetryLimit      int    `envconfig:"FORMLANDER_WEBHOOK_RETRY_LIMIT" default:"3"`
	BackoffSchedule string `envconfig:"FORMLANDER_WEBHOOK_BACKOFF" default:"1,5,15,60"`
}

var (
	cfgOnce sync.Once
	cfgInst *Config
)

// Get returns the singleton configuration instance populated from environment variables.
func Get() *Config {
	cfgOnce.Do(func() {
		cfgInst = &Config{}
		if err := envconfig.Process("", cfgInst); err != nil {
			log.Fatalf("config: failed to process environment variables: %v", err)
		}

		cfgInst.DatabasePath = cfgInst.resolveDatabasePath()
		cfgInst.ensureDirectories()

		if err := cfgInst.Validate(); err != nil {
			log.Fatalf("config: invalid configuration: %v", err)
		}
	})
	return cfgInst
}

func (c *Config) Validate() error {
	var problems []string

	// In production, REQUIRE session secret and anon salt
	if c.IsProduction() {
		if c.SessionSecret == "" {
			problems = append(problems, "FORMLANDER_SESSION_SECRET is REQUIRED in production (generate with: openssl rand -hex 32)")
		}
		if c.AnonSalt == "" {
			problems = append(problems, "FORMLANDER_ANON_SALT is REQUIRED in production (generate with: openssl rand -hex 32)")
		}
	} else {
		// Auto-generate secrets in non-production (with warnings)
		if c.SessionSecret == "" {
			c.SessionSecret = generateSecret()
			log.Println("⚠️  FORMLANDER_SESSION_SECRET not set - generated random secret (sessions will be invalidated on restart)")
		}
		if c.AnonSalt == "" {
			c.AnonSalt = generateSecret()
			log.Println("⚠️  FORMLANDER_ANON_SALT not set - generated random salt (IP hashes will change on restart)")
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
