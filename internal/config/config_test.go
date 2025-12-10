package config

import (
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	// Clean environment before test
	defer Reset()
	os.Clearenv()
	os.Setenv("FORMLANDER_ENV", "development") // Use development to avoid session secret requirement

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}

	// Check defaults
	if cfg.Environment != "development" {
		t.Errorf("Expected Environment=development, got %s", cfg.Environment)
	}
	if cfg.Port != "8080" {
		t.Errorf("Expected Port=8080, got %s", cfg.Port)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("Expected LogLevel=error, got %s", cfg.LogLevel)
	}
	if cfg.DataDirectory != "storage" {
		t.Errorf("Expected DataDirectory=storage, got %s", cfg.DataDirectory)
	}
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	defer Reset()
	os.Clearenv()

	tests := []struct {
		name     string
		envVar   string
		envValue string
		setup    func()
		check    func(*Config) error
	}{
		{
			name:     "FORMLANDER_ENV",
			envVar:   "FORMLANDER_ENV",
			envValue: "development",
			setup:    func() {},
			check: func(c *Config) error {
				if c.Environment != "development" {
					t.Errorf("Expected Environment=development, got %s", c.Environment)
				}
				return nil
			},
		},
		{
			name:     "FORMLANDER_PORT",
			envVar:   "FORMLANDER_PORT",
			envValue: "3000",
			setup: func() {
				os.Setenv("FORMLANDER_ENV", "development")
			},
			check: func(c *Config) error {
				if c.Port != "3000" {
					t.Errorf("Expected Port=3000, got %s", c.Port)
				}
				return nil
			},
		},
		{
			name:     "FORMLANDER_LOG_LEVEL",
			envVar:   "FORMLANDER_LOG_LEVEL",
			envValue: "debug",
			setup: func() {
				os.Setenv("FORMLANDER_ENV", "development")
			},
			check: func(c *Config) error {
				if c.LogLevel != "debug" {
					t.Errorf("Expected LogLevel=debug, got %s", c.LogLevel)
				}
				return nil
			},
		},
		{
			name:     "FORMLANDER_DATA_DIR",
			envVar:   "FORMLANDER_DATA_DIR",
			envValue: "/custom/path",
			setup: func() {
				os.Setenv("FORMLANDER_ENV", "development")
			},
			check: func(c *Config) error {
				if c.DataDirectory != "/custom/path" {
					t.Errorf("Expected DataDirectory=/custom/path, got %s", c.DataDirectory)
				}
				return nil
			},
		},
		{
			name:     "FORMLANDER_SESSION_SECRET",
			envVar:   "FORMLANDER_SESSION_SECRET",
			envValue: "custom-secret-123",
			setup: func() {
				os.Setenv("FORMLANDER_ENV", "production")
			},
			check: func(c *Config) error {
				if c.SessionSecret != "custom-secret-123" {
					t.Errorf("Expected SessionSecret=custom-secret-123, got %s", c.SessionSecret)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Reset()
			os.Clearenv()
			tt.setup()
			os.Setenv(tt.envVar, tt.envValue)

			cfg := Get()
			tt.check(cfg)
		})
	}
}

func TestSessionSecretProductionRequired(t *testing.T) {
	defer Reset()
	os.Clearenv()
	os.Setenv("FORMLANDER_ENV", "production")
	os.Setenv("FORMLANDER_SESSION_SECRET", "required-secret")

	cfg := Get()
	if cfg.SessionSecret != "required-secret" {
		t.Errorf("Expected SessionSecret=required-secret in production, got %s", cfg.SessionSecret)
	}
}

func TestSessionSecretDevelopment(t *testing.T) {
	defer Reset()
	os.Clearenv()
	os.Setenv("FORMLANDER_ENV", "development")

	cfg := Get()
	if cfg.SessionSecret == "" {
		t.Error("Expected SessionSecret to be auto-generated in development")
	}
	if cfg.SessionSecret != "dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4" {
		t.Errorf("Expected fixed dev secret, got %s", cfg.SessionSecret)
	}
}

func TestSessionSecretTest(t *testing.T) {
	defer Reset()
	os.Clearenv()
	os.Setenv("FORMLANDER_ENV", "test")

	cfg := Get()
	if cfg.SessionSecret == "" {
		t.Error("Expected SessionSecret to be auto-generated in test")
	}
	if cfg.SessionSecret != "dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4" {
		t.Errorf("Expected fixed dev secret, got %s", cfg.SessionSecret)
	}
}

func TestIsProduction(t *testing.T) {
	defer Reset()
	os.Clearenv()

	tests := []struct {
		env        string
		wantProd   bool
		wantDev    bool
		wantTest   bool
	}{
		{"production", true, false, false},
		{"development", false, true, false},
		{"test", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			Reset()
			os.Clearenv()
			os.Setenv("FORMLANDER_ENV", tt.env)
			os.Setenv("FORMLANDER_SESSION_SECRET", "test-secret")

			cfg := Get()
			if cfg.IsProduction() != tt.wantProd {
				t.Errorf("IsProduction() = %v, want %v", cfg.IsProduction(), tt.wantProd)
			}
			if cfg.IsDevelopment() != tt.wantDev {
				t.Errorf("IsDevelopment() = %v, want %v", cfg.IsDevelopment(), tt.wantDev)
			}
			if cfg.IsTest() != tt.wantTest {
				t.Errorf("IsTest() = %v, want %v", cfg.IsTest(), tt.wantTest)
			}
		})
	}
}

func TestDatabasePath(t *testing.T) {
	defer Reset()
	os.Clearenv()

	tests := []struct {
		name     string
		env      string
		filename string
		wantPath string
	}{
		{
			name:     "development environment",
			env:      "development",
			filename: "formlander.db",
			wantPath: "storage/formlander.development.db",
		},
		{
			name:     "production environment",
			env:      "production",
			filename: "formlander.db",
			wantPath: "storage/formlander.production.db",
		},
		{
			name:     "test environment",
			env:      "test",
			filename: "formlander.db",
			wantPath: "storage/formlander.test.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Reset()
			os.Clearenv()
			os.Setenv("FORMLANDER_ENV", tt.env)
			os.Setenv("FORMLANDER_SESSION_SECRET", "test-secret")

			cfg := Get()
			if cfg.DatabasePath != tt.wantPath {
				t.Errorf("DatabasePath = %v, want %v", cfg.DatabasePath, tt.wantPath)
			}
		})
	}
}

func TestGetMaxOpenConns(t *testing.T) {
	defer Reset()
	os.Clearenv()

	tests := []struct {
		name string
		env  string
		want int
	}{
		{"production", "production", 10},
		{"development", "development", 1},
		{"test", "test", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Reset()
			os.Clearenv()
			os.Setenv("FORMLANDER_ENV", tt.env)
			os.Setenv("FORMLANDER_SESSION_SECRET", "test-secret")

			cfg := Get()
			got := cfg.GetMaxOpenConns()
			if got != tt.want {
				t.Errorf("GetMaxOpenConns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetMaxIdleConns(t *testing.T) {
	defer Reset()
	os.Clearenv()

	tests := []struct {
		name string
		env  string
		want int
	}{
		{"production", "production", 5},
		{"development", "development", 1},
		{"test", "test", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Reset()
			os.Clearenv()
			os.Setenv("FORMLANDER_ENV", tt.env)
			os.Setenv("FORMLANDER_SESSION_SECRET", "test-secret")

			cfg := Get()
			got := cfg.GetMaxIdleConns()
			if got != tt.want {
				t.Errorf("GetMaxIdleConns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebhookBackoff(t *testing.T) {
	defer Reset()
	os.Clearenv()
	os.Setenv("FORMLANDER_ENV", "development")

	cfg := Get()
	backoff := cfg.WebhookBackoff()

	expected := []int{1, 5, 15, 60}
	if len(backoff) != len(expected) {
		t.Errorf("WebhookBackoff() length = %v, want %v", len(backoff), len(expected))
	}

	for i, v := range expected {
		if backoff[i] != v {
			t.Errorf("WebhookBackoff()[%d] = %v, want %v", i, backoff[i], v)
		}
	}
}

func TestInvalidEnvironmentValidation(t *testing.T) {
	// Skip test that would call os.Exit via log.Fatalf
	t.Skip("Skipping test that would call os.Exit - invalid environment causes log.Fatalf")
}

func TestReset(t *testing.T) {
	defer Reset()
	os.Clearenv()
	os.Setenv("FORMLANDER_ENV", "development")

	cfg1 := Get()
	if cfg1 == nil {
		t.Fatal("First Get() returned nil")
	}

	Reset()
	os.Setenv("FORMLANDER_ENV", "development")

	cfg2 := Get()
	if cfg2 == nil {
		t.Fatal("Second Get() after Reset() returned nil")
	}

	// Should be different instances after reset
	if cfg1 == cfg2 {
		t.Error("Expected different config instances after Reset()")
	}
}
