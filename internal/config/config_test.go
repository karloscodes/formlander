package config

import (
	"os"
	"testing"
)

func TestConfig(t *testing.T) {
	t.Run("Get returns config with defaults", func(t *testing.T) {
		defer Reset()
		os.Clearenv()
		os.Setenv("FORMLANDER_ENV", "development")

		cfg := Get()
		if cfg == nil {
			t.Fatal("Get() returned nil")
		}

		if cfg.Environment != "development" {
			t.Errorf("Expected Environment=development, got %s", cfg.Environment)
		}
		if cfg.Port != "8080" {
			t.Errorf("Expected Port=8080, got %s", cfg.Port)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("Expected LogLevel=info (development default), got %s", cfg.LogLevel)
		}
		if cfg.DataDirectory != "storage" {
			t.Errorf("Expected DataDirectory=storage, got %s", cfg.DataDirectory)
		}
	})

	t.Run("Reset creates new config instance", func(t *testing.T) {
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

		if cfg1 == cfg2 {
			t.Error("Expected different config instances after Reset()")
		}
	})
}

func TestEnvironmentVariableOverrides(t *testing.T) {
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

func TestSessionSecret(t *testing.T) {
	t.Run("production requires secret", func(t *testing.T) {
		Reset()
		os.Clearenv()
		os.Setenv("FORMLANDER_ENV", "production")
		os.Setenv("FORMLANDER_SESSION_SECRET", "required-secret")

		cfg := Get()
		if cfg.SessionSecret != "required-secret" {
			t.Errorf("Expected SessionSecret=required-secret in production, got %s", cfg.SessionSecret)
		}
	})

	t.Run("development uses fixed dev secret", func(t *testing.T) {
		Reset()
		os.Clearenv()
		os.Setenv("FORMLANDER_ENV", "development")

		cfg := Get()
		if cfg.SessionSecret == "" {
			t.Error("Expected SessionSecret to be auto-generated in development")
		}
		if cfg.SessionSecret != "dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4" {
			t.Errorf("Expected fixed dev secret, got %s", cfg.SessionSecret)
		}
	})

	t.Run("test uses fixed dev secret", func(t *testing.T) {
		Reset()
		os.Clearenv()
		os.Setenv("FORMLANDER_ENV", "test")

		cfg := Get()
		if cfg.SessionSecret == "" {
			t.Error("Expected SessionSecret to be auto-generated in test")
		}
		if cfg.SessionSecret != "dev-secret-do-not-use-in-production-f8e3a9c2d1b7e6a4" {
			t.Errorf("Expected fixed dev secret, got %s", cfg.SessionSecret)
		}
	})
}

func TestIsEnvironment(t *testing.T) {
	tests := []struct {
		env      string
		wantProd bool
		wantDev  bool
		wantTest bool
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
	tests := []struct {
		name     string
		env      string
		wantPath string
	}{
		{"development environment", "development", "storage/formlander.development.db"},
		{"production environment", "production", "storage/formlander.production.db"},
		{"test environment", "test", "storage/formlander.test.db"},
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

func TestConnectionPooling(t *testing.T) {
	tests := []struct {
		env          string
		wantMaxOpen  int
		wantMaxIdle  int
	}{
		{"production", 10, 5},
		{"development", 1, 1},
		{"test", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			Reset()
			os.Clearenv()
			os.Setenv("FORMLANDER_ENV", tt.env)
			os.Setenv("FORMLANDER_SESSION_SECRET", "test-secret")

			cfg := Get()
			if cfg.GetMaxOpenConns() != tt.wantMaxOpen {
				t.Errorf("GetMaxOpenConns() = %v, want %v", cfg.GetMaxOpenConns(), tt.wantMaxOpen)
			}
			if cfg.GetMaxIdleConns() != tt.wantMaxIdle {
				t.Errorf("GetMaxIdleConns() = %v, want %v", cfg.GetMaxIdleConns(), tt.wantMaxIdle)
			}
		})
	}
}

func TestWebhookBackoff(t *testing.T) {
	t.Run("returns expected backoff schedule", func(t *testing.T) {
		Reset()
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
	})
}
