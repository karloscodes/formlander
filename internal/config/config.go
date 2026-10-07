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

// defaultSessionTimeout is how long a login lasts, in seconds: 90 days.
const defaultSessionTimeout = 90 * 24 * 60 * 60

var (
	cfgOnce sync.Once
	cfgInst *Config
)

// Get returns the singleton configuration instance.
func Get() *Config {
	cfgOnce.Do(func() {
		loadDotEnv()

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

		// A login lasts 90 days, unless the environment sets another time.
		// Cartridge does not bind this variable, so it is read here.
		base.SessionTimeout = defaultSessionTimeout
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

// loadDotEnv copies the FORMLANDER_ settings of a .env file in the working
// directory into the environment, unless the environment already sets them.
// Cartridge reads FORMLANDER_ENV and the session secret only from the
// environment, so without this a .env file could not turn on production mode,
// and the rate limits and the Secure cookie stayed off.
func loadDotEnv() {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	if v.ReadInConfig() != nil {
		return
	}
	for _, key := range v.AllKeys() {
		name := strings.ToUpper(key)
		if strings.HasPrefix(name, "FORMLANDER_") && os.Getenv(name) == "" {
			os.Setenv(name, v.GetString(key))
		}
	}
}

// publicSecrets are session secrets anyone can read in the source code or in
// its documentation. A session signed with one of them can be forged.
var publicSecrets = map[string]bool{
	"dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4": true,
	"replace-me-with-random-secret":                        true,
	"replace-me-session-secret":                            true,
	"your-saved-secret-here":                               true,
	"your-secret-here":                                     true,
}

// minSecretLength is the shortest session secret Formlander accepts. A shorter
// one is a placeholder or can be guessed.
const minSecretLength = 32

// SessionSecretFile holds the generated secret in the data directory, so
// sessions survive a restart.
const SessionSecretFile = "session-secret"

// ensureSessionSecret replaces a missing, public, or short session secret with
// a random one stored in the data directory. The test environment keeps the
// fixed secret. If the file cannot be written, the secret lives in memory
// and sessions end at the next restart.
func ensureSessionSecret(c *config.Config) {
	if c.IsTest() || (len(c.SessionSecret) >= minSecretLength && !publicSecrets[c.SessionSecret]) {
		return
	}
	if c.SessionSecret != "" {
		log.Printf("warn: the session secret is public or shorter than %d characters, so Formlander uses a random one", minSecretLength)
	}

	path := filepath.Join(c.DataDirectory, SessionSecretFile)
	if data, err := os.ReadFile(path); err == nil {
		if stored := strings.TrimSpace(string(data)); len(stored) >= minSecretLength {
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
