package config

import (
	"crypto/rand"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/karloscodes/cartridge/config"
	"github.com/spf13/viper"
)

const appName = "formlander"

// Config extends cartridge config with formlander-specific settings.
type Config struct {
	*config.Config

	// Form limits.
	MaxInputFields int `mapstructure:"maxinputfields"`

	// Webhook configuration.
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

// Get returns the singleton configuration instance.
func Get() *Config {
	cfgOnce.Do(func() {
		// Default FORMLANDER_ENV to development; production is opt-in.
		// Cartridge defaults to production, which marks the session cookie
		// Secure and breaks login on plain-HTTP self-hosted deploys. Set
		// this before config.Load so cartridge's production-mode validation
		// (e.g. SESSION_SECRET required) doesn't fire on an empty env.
		if os.Getenv("FORMLANDER_ENV") == "" {
			os.Setenv("FORMLANDER_ENV", config.Development)
		}

		// Load base cartridge config
		base, err := config.Load(appName)
		if err != nil {
			log.Fatalf("config: %v", err)
		}

		// Load formlander-specific config
		v := viper.New()
		v.SetConfigName(".env")
		v.SetConfigType("env")
		v.AddConfigPath(".")
		_ = v.ReadInConfig()

		// Set formlander-specific defaults
		v.SetDefault("maxinputfields", 200)
		v.SetDefault("webhook.signatureheader", "X-Formlander-Signature")
		v.SetDefault("webhook.retrylimit", 3)
		v.SetDefault("webhook.backoffschedule", "1,5,15,60")

		// Cartridge does not bind this variable, so the Dockerfile value
		// was ignored and sessions lasted cartridge's 7-day default.
		if raw := os.Getenv("FORMLANDER_SESSION_TIMEOUT_SECONDS"); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
				base.SessionTimeout = seconds
			}
		}

		ensureSessionSecret(base)

		cfgInst = &Config{Config: base}
		if err := v.Unmarshal(cfgInst); err != nil {
			log.Fatalf("config: failed to unmarshal: %v", err)
		}
	})
	return cfgInst
}

// publicSecrets are session secrets anyone can read in the source code.
// A session signed with one of them can be forged.
var publicSecrets = map[string]bool{
	"dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4": true,
	"replace-me-with-random-secret":                        true,
}

// SessionSecretFile holds the generated secret in the data directory, so
// sessions survive a restart.
const SessionSecretFile = "session-secret"

// ensureSessionSecret replaces a missing or public session secret with a
// random one stored in the data directory. The test environment keeps the
// fixed secret. If the file cannot be written, the secret lives in memory
// and sessions end at the next restart.
func ensureSessionSecret(c *config.Config) {
	if c.IsTest() || (c.SessionSecret != "" && !publicSecrets[c.SessionSecret]) {
		return
	}

	path := filepath.Join(c.DataDirectory, SessionSecretFile)
	if data, err := os.ReadFile(path); err == nil {
		if stored := strings.TrimSpace(string(data)); len(stored) >= 32 {
			c.SessionSecret = stored
			return
		}
	}

	c.SessionSecret = rand.Text() + rand.Text()
	if err := os.MkdirAll(c.DataDirectory, 0o755); err != nil {
		log.Printf("warn: cannot store the session secret, sessions end at restart: %v", err)
		return
	}
	if err := os.WriteFile(path, []byte(c.SessionSecret+"\n"), 0o600); err != nil {
		log.Printf("warn: cannot store the session secret, sessions end at restart: %v", err)
	}
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

// Reset clears the cached configuration; intended for tests.
func Reset() {
	cfgOnce = sync.Once{}
	cfgInst = nil
}
