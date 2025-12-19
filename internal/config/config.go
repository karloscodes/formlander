package config

import (
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

	// Uploads configuration.
	UploadsDirectory string `mapstructure:"uploadsdirectory"`
	MaxUploadSizeMB  int    `mapstructure:"maxuploadssizemb"`
	MaxInputFields   int    `mapstructure:"maxinputfields"`
	MaxPayloadSizeMB int    `mapstructure:"maxpayloadsizemb"`

	// Rate limiting.
	SubmissionRatePerHour int `mapstructure:"submissionrateperhour"`

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
		v.SetDefault("uploadsdirectory", "uploads")
		v.SetDefault("maxuploadssizemb", 10)
		v.SetDefault("maxinputfields", 200)
		v.SetDefault("maxpayloadsizemb", 2)
		v.SetDefault("submissionrateperhour", 120)
		v.SetDefault("webhook.signatureheader", "X-Formlander-Signature")
		v.SetDefault("webhook.retrylimit", 3)
		v.SetDefault("webhook.backoffschedule", "1,5,15,60")

		cfgInst = &Config{Config: base}
		if err := v.Unmarshal(cfgInst); err != nil {
			log.Fatalf("config: failed to unmarshal: %v", err)
		}

		// Ensure uploads directory exists
		if uploads := cfgInst.GetUploadsDirectory(); uploads != "" {
			if err := os.MkdirAll(uploads, 0o755); err != nil {
				log.Printf("config: failed to create uploads directory %q: %v", uploads, err)
			}
		}
	})
	return cfgInst
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

// Reset clears the cached configuration; intended for tests.
func Reset() {
	cfgOnce = sync.Once{}
	cfgInst = nil
}
