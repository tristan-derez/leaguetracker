package utils

import (
	"fmt"
	"time"
)

// RetryConfig holds the configuration for the retry mechanism
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig provides sensible default values for RetryConfig
var DefaultRetryConfig = RetryConfig{
	MaxRetries: 5,
	BaseDelay:  time.Second,
	MaxDelay:   30 * time.Second,
}

// RetryWithBackoff attempts to execute the given function with exponential backoff
func RetryWithBackoff(operation func() error, config RetryConfig) error {
	var err error
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		err = operation()
		if err == nil {
			return nil // Success, exit the function
		}

		if attempt == config.MaxRetries-1 {
			break // Last attempt, exit the loop
		}

		delay := calculateBackoff(attempt, config.BaseDelay, config.MaxDelay)
		time.Sleep(delay)
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries, err)
}

// NonRetryableError represents an error that should not be retried
type NonRetryableError struct {
	Err error
}

// Error returns the error message for the NonRetryableError
func (e *NonRetryableError) Error() string {
	return fmt.Sprintf("non-retryable error: %v", e.Err)
}

// Unwrap returns the underlying error, allowing error unwrapping
func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

// NewNonRetryableError creates a new NonRetryableError
func NewNonRetryableError(err error) *NonRetryableError {
	return &NonRetryableError{Err: err}
}

// NewNonRetryableErrorf creates a new NonRetryableError with a formatted message
func NewNonRetryableErrorf(format string, a ...interface{}) *NonRetryableError {
	return &NonRetryableError{Err: fmt.Errorf(format, a...)}
}

func calculateBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	delay := baseDelay * (1 << attempt) // Exponential backoff
	return minDuration(delay, maxDelay)
}

// MinDuration returns the smaller of two time.Duration values
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
