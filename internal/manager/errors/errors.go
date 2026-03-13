package errors

import (
	"fmt"
	"math"
	"time"
)

// ValidationError represents validation-specific errors
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("validation failed for field '%s' with value '%s': %s", e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
}

// DockerError represents Docker-specific errors
type DockerError struct {
	Operation string
	Container string
	Err       error
}

func (e *DockerError) Error() string {
	if e.Container != "" {
		return fmt.Sprintf("docker operation '%s' failed for container '%s': %v", e.Operation, e.Container, e.Err)
	}
	return fmt.Sprintf("docker operation '%s' failed: %v", e.Operation, e.Err)
}

func (e *DockerError) Unwrap() error {
	return e.Err
}

// ConfigError represents configuration-related errors
type ConfigError struct {
	Field   string
	Value   string
	Message string
}

func (e *ConfigError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("config error for field '%s' with value '%s': %s", e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("config error for field '%s': %s", e.Field, e.Message)
}

// WrapWithContext wraps an error with additional context
func WrapWithContext(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// RetryWithBackoff executes an operation with exponential backoff retry logic
func RetryWithBackoff(operation func() error, maxRetries int, baseDelay time.Duration) error {
	if maxRetries <= 0 {
		return fmt.Errorf("maxRetries must be greater than 0")
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
			if i == maxRetries-1 {
				break
			}
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(i)))
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}

// NewValidationError creates a new validation error
func NewValidationError(field, value, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// NewDockerError creates a new Docker error
func NewDockerError(operation, container string, err error) *DockerError {
	return &DockerError{
		Operation: operation,
		Container: container,
		Err:       err,
	}
}

// NewConfigError creates a new configuration error
func NewConfigError(field, value, message string) *ConfigError {
	return &ConfigError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}
