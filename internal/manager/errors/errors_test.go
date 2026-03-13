package errors

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestValidationError(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		message  string
		expected string
	}{
		{
			name:     "with value",
			field:    "email",
			value:    "invalid-email",
			message:  "invalid format",
			expected: "validation failed for field 'email' with value 'invalid-email': invalid format",
		},
		{
			name:     "without value",
			field:    "password",
			value:    "",
			message:  "too short",
			expected: "validation failed for field 'password': too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ValidationError{
				Field:   tt.field,
				Value:   tt.value,
				Message: tt.message,
			}
			if got := err.Error(); got != tt.expected {
				t.Errorf("ValidationError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDockerError(t *testing.T) {
	originalErr := fmt.Errorf("container not found")

	tests := []struct {
		name      string
		operation string
		container string
		err       error
		expected  string
	}{
		{
			name:      "with container",
			operation: "start",
			container: "my-app",
			err:       originalErr,
			expected:  "docker operation 'start' failed for container 'my-app': container not found",
		},
		{
			name:      "without container",
			operation: "pull",
			container: "",
			err:       originalErr,
			expected:  "docker operation 'pull' failed: container not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dockerErr := &DockerError{
				Operation: tt.operation,
				Container: tt.container,
				Err:       tt.err,
			}

			if got := dockerErr.Error(); got != tt.expected {
				t.Errorf("DockerError.Error() = %v, want %v", got, tt.expected)
			}

			if unwrapped := dockerErr.Unwrap(); unwrapped != originalErr {
				t.Errorf("DockerError.Unwrap() = %v, want %v", unwrapped, originalErr)
			}
		})
	}
}

func TestConfigError(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		message  string
		expected string
	}{
		{
			name:     "with value",
			field:    "domain",
			value:    "invalid.domain",
			message:  "invalid format",
			expected: "config error for field 'domain' with value 'invalid.domain': invalid format",
		},
		{
			name:     "without value",
			field:    "password",
			value:    "",
			message:  "cannot be empty",
			expected: "config error for field 'password': cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ConfigError{
				Field:   tt.field,
				Value:   tt.value,
				Message: tt.message,
			}
			if got := err.Error(); got != tt.expected {
				t.Errorf("ConfigError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWrapWithContext(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		context  string
		expected string
	}{
		{
			name:     "with error",
			err:      fmt.Errorf("original error"),
			context:  "operation failed",
			expected: "operation failed: original error",
		},
		{
			name:     "with nil error",
			err:      nil,
			context:  "operation failed",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapWithContext(tt.err, tt.context)
			if tt.err == nil {
				if result != nil {
					t.Errorf("WrapWithContext() with nil error should return nil, got %v", result)
				}
				return
			}

			if got := result.Error(); got != tt.expected {
				t.Errorf("WrapWithContext() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("success on first try", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return nil
		}

		err := RetryWithBackoff(operation, 3, 10*time.Millisecond)
		if err != nil {
			t.Errorf("RetryWithBackoff() should succeed on first try, got error: %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("success on second try", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts == 1 {
				return fmt.Errorf("temporary error")
			}
			return nil
		}

		err := RetryWithBackoff(operation, 3, 10*time.Millisecond)
		if err != nil {
			t.Errorf("RetryWithBackoff() should succeed on second try, got error: %v", err)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("failure after max retries", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return fmt.Errorf("persistent error")
		}

		err := RetryWithBackoff(operation, 3, 10*time.Millisecond)
		if err == nil {
			t.Error("RetryWithBackoff() should fail after max retries")
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		expectedMsg := "operation failed after 3 retries: persistent error"
		if got := err.Error(); got != expectedMsg {
			t.Errorf("Error message = %v, want %v", got, expectedMsg)
		}
	})

	t.Run("invalid max retries", func(t *testing.T) {
		operation := func() error {
			return nil
		}

		err := RetryWithBackoff(operation, 0, 10*time.Millisecond)
		if err == nil {
			t.Error("RetryWithBackoff() should fail with invalid maxRetries")
		}

		expectedMsg := "maxRetries must be greater than 0"
		if got := err.Error(); got != expectedMsg {
			t.Errorf("Error message = %v, want %v", got, expectedMsg)
		}
	})
}

func TestErrorConstructors(t *testing.T) {
	t.Run("NewValidationError", func(t *testing.T) {
		err := NewValidationError("email", "test@example", "invalid format")
		if err.Field != "email" || err.Value != "test@example" || err.Message != "invalid format" {
			t.Error("NewValidationError did not set fields correctly")
		}
	})

	t.Run("NewDockerError", func(t *testing.T) {
		originalErr := fmt.Errorf("docker error")
		err := NewDockerError("run", "my-container", originalErr)
		if err.Operation != "run" || err.Container != "my-container" || err.Err != originalErr {
			t.Error("NewDockerError did not set fields correctly")
		}
	})

	t.Run("NewConfigError", func(t *testing.T) {
		err := NewConfigError("domain", "example.com", "invalid")
		if err.Field != "domain" || err.Value != "example.com" || err.Message != "invalid" {
			t.Error("NewConfigError did not set fields correctly")
		}
	})
}

func TestErrorsAs(t *testing.T) {
	validationErr := NewValidationError("test", "value", "invalid")

	var target *ValidationError
	if !errors.As(validationErr, &target) {
		t.Error("ValidationError should be unwrappable with errors.As")
	}

	if target.Field != "test" {
		t.Errorf("Unwrapped error field = %v, want %v", target.Field, "test")
	}
}
