package riotapi

import (
	"fmt"
	"net/http"
	"strings"
)

// RiotAPIError represents a custom error type for Riot API responses
type RiotAPIError struct {
	StatusCode int
	Message    string
	Headers    http.Header
}

// Error implements the error interface for RiotAPIError
func (e *RiotAPIError) Error() string {
	return fmt.Sprintf("Riot API error (status %d): %s", e.StatusCode, e.Message)
}

// IsRateLimitError checks if the error is a rate limit error based on Riot API documentation
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	if apiErr, ok := err.(*RiotAPIError); ok {
		// Check for 429 status code
		if apiErr.StatusCode == http.StatusTooManyRequests {
			// Check for X-Rate-Limit-Type header
			if rateLimitType := apiErr.Headers.Get("X-Rate-Limit-Type"); rateLimitType != "" {
				return true
			}
			// If no X-Rate-Limit-Type header, it might be a service-specific rate limit
			return strings.Contains(strings.ToLower(apiErr.Message), "rate limit")
		}
	}

	// Fallback to checking error message for non-RiotAPIError types
	return strings.Contains(strings.ToLower(err.Error()), "rate limit")
}
