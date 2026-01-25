package product

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var defaultConfigJSON = []byte(`{
  "product_name": "Formlander",
  "binary_name": "formlander",
  "slug": "formlander",
  "github_repo": "karloscodes/formlander",
  "domain_example": "forms.example.com",
  "admin_email_prefix": "admin-formlander",
  "app_image": "karloscodes/formlander:latest",
  "pro_app_image": "karloscodes/formlander-pro:latest",
  "caddy_image": "caddy:2.9-alpine",
  "release_binary_prefix": "formlander",
  "legacy_binary_prefix": "formlander",
  "license_required": false
}`)

var (
	settings     *Settings
	settingsOnce sync.Once
)

// Settings captures all of the product-specific values that shape the installer.
type Settings struct {
	ProductName          string `json:"product_name"`
	BinaryName           string `json:"binary_name"`
	Slug                 string `json:"slug"`
	GitHubRepo           string `json:"github_repo"`
	DomainExample        string `json:"domain_example"`
	AdminEmailPrefix     string `json:"admin_email_prefix"`
	AppImage             string `json:"app_image"`
	ProAppImage          string `json:"pro_app_image"`
	CaddyImage           string `json:"caddy_image"`
	ReleaseBinaryPrefix  string `json:"release_binary_prefix"`
	LegacyBinaryPrefix   string `json:"legacy_binary_prefix"`
	InstallDir           string `json:"install_dir"`
	BinaryPath           string `json:"binary_path"`
	CronFile             string `json:"cron_file"`
	CronSchedule         string `json:"cron_schedule"`
	LogDir               string `json:"log_dir"`
	CLIlogFile           string `json:"cli_log_file"`
	UpdaterLogFile       string `json:"updater_log_file"`
	ReloaderLogFile      string `json:"reloader_log_file"`
	NetworkName          string `json:"network_name"`
	CaddyContainer       string `json:"caddy_container"`
	AppPrimaryContainer  string `json:"app_primary_container"`
	AppSecondaryContainer string `json:"app_secondary_container"`
	EnvPrefix            string `json:"env_prefix"`
	DatabaseFileName     string `json:"database_file_name"`
	LicenseRequired      bool   `json:"license_required"`
}

// Get returns the initialized Settings instance (loading it on first use).
func Get() *Settings {
	settingsOnce.Do(func() {
		settings = loadSettings()
	})
	return settings
}

// loadSettings resolves the configuration bytes and applies defaults.
func loadSettings() *Settings {
	data := loadConfigBytes()

	var cfg Settings
	if err := json.Unmarshal(data, &cfg); err != nil {
		panic(fmt.Errorf("failed to parse product config: %w", err))
	}

	cfg.applyDefaults()
	return &cfg
}

func loadConfigBytes() []byte {
	// Highest priority: explicit path via PRODUCT_CONFIG_PATH
	if path := os.Getenv("PRODUCT_CONFIG_PATH"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
	}

	// Next: local config/config.json relative to current working directory
	if data, err := os.ReadFile(filepath.Clean("config/config.json")); err == nil {
		return data
	}

	// Fallback to embedded config baked into the binary.
	return defaultConfigJSON
}

func (s *Settings) applyDefaults() {
	s.Slug = firstNonEmpty(s.Slug, s.BinaryName, "app")
	if s.BinaryName == "" {
		s.BinaryName = s.Slug
	}
	if s.ProductName == "" {
		s.ProductName = titleCase(s.Slug)
	}
	if s.EnvPrefix == "" {
		s.EnvPrefix = strings.ToUpper(strings.ReplaceAll(s.Slug, "-", "_"))
	} else {
		s.EnvPrefix = strings.ToUpper(strings.ReplaceAll(s.EnvPrefix, "-", "_"))
	}
	if s.InstallDir == "" {
		s.InstallDir = filepath.Join("/opt", s.Slug)
	}
	if s.BinaryPath == "" {
		s.BinaryPath = filepath.Join("/usr/local/bin", s.BinaryName)
	}
	if s.CronFile == "" {
		s.CronFile = fmt.Sprintf("/etc/cron.d/%s-update", s.Slug)
	}
	if s.CronSchedule == "" {
		s.CronSchedule = "0 3 * * *"
	}
	if s.LogDir == "" {
		s.LogDir = filepath.Join(s.InstallDir, "logs")
	}
	if s.CLIlogFile == "" {
		s.CLIlogFile = fmt.Sprintf("%s-cli.log", s.Slug)
	}
	if s.UpdaterLogFile == "" {
		s.UpdaterLogFile = fmt.Sprintf("%s-updater.log", s.Slug)
	}
	if s.ReloaderLogFile == "" {
		s.ReloaderLogFile = fmt.Sprintf("%s-reloader.log", s.Slug)
	}
	if s.NetworkName == "" {
		s.NetworkName = fmt.Sprintf("%s-network", s.Slug)
	}
	if s.CaddyContainer == "" {
		s.CaddyContainer = fmt.Sprintf("%s-caddy", s.Slug)
	}
	if s.AppPrimaryContainer == "" {
		s.AppPrimaryContainer = fmt.Sprintf("%s-app-1", s.Slug)
	}
	if s.AppSecondaryContainer == "" {
		s.AppSecondaryContainer = fmt.Sprintf("%s-app-2", s.Slug)
	}
	if s.DatabaseFileName == "" {
		s.DatabaseFileName = fmt.Sprintf("%s-production.db", s.Slug)
	}
	if s.AdminEmailPrefix == "" {
		s.AdminEmailPrefix = fmt.Sprintf("admin-%s", s.Slug)
	}
	if s.ReleaseBinaryPrefix == "" {
		s.ReleaseBinaryPrefix = fmt.Sprintf("%s-installer", s.Slug)
	}
	if s.LegacyBinaryPrefix == "" {
		s.LegacyBinaryPrefix = s.Slug
	}
	if s.GitHubRepo == "" {
		s.GitHubRepo = "karloscodes/formlander-installer"
	}
	if s.DomainExample == "" {
		s.DomainExample = fmt.Sprintf("%s.example.com", s.Slug)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

// EnvKey formats an installer environment key using the configured prefix.
func (s *Settings) EnvKey(suffix string) string {
	suffix = strings.ToUpper(strings.ReplaceAll(suffix, "-", "_"))
	return fmt.Sprintf("%s_%s", s.EnvPrefix, suffix)
}

func (s *Settings) DomainEnvKey() string {
	return s.EnvKey("DOMAIN")
}

func (s *Settings) PrivateKeyEnvKey() string {
	return s.EnvKey("PRIVATE_KEY")
}

func (s *Settings) LicenseKeyEnvKey() string {
	return s.EnvKey("LICENSE_KEY")
}

func (s *Settings) UserEnvKey() string {
	return s.EnvKey("USER")
}

func (s *Settings) AppBaseContainer() string {
	return fmt.Sprintf("%s-app", s.Slug)
}

func (s *Settings) InstallerURL() string {
	return fmt.Sprintf("https://github.com/%s/releases/latest", s.GitHubRepo)
}

func (s *Settings) AdminEmail(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return s.AdminEmailPrefix + "@localhost"
	}
	return fmt.Sprintf("%s@%s", s.AdminEmailPrefix, domain)
}
