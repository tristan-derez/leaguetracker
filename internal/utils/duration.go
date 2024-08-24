package utils

import "time"

// MinDuration returns the smaller of two time.Duration values
func MinDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
