package riotapi

import (
	"context"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a new RateLimiter with specified rate and burst size.
// requestsPerSecond: The number of requests allowed per second.
// burstSize: The maximum number of requests allowed to be made at once.
func NewRateLimiter(requestsPerSecond float64, burstSize int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), burstSize),
	}
}

// Wait blocks until a request can be made without exceeding the rate limit.
// It uses a background context, which means it will wait indefinitely if necessary.
func (rl *RateLimiter) Wait() {
	rl.limiter.Wait(context.Background())
}
