package ratelimit

import (
	"context"
	"time"
)

// Limiter provides rate limiting functionality
type Limiter interface {
	// Allow checks if an action is allowed under the rate limit
	// Returns true if allowed, false if rate limit exceeded
	Allow(ctx context.Context, key string) (bool, error)

	// AllowN checks if N actions are allowed under the rate limit
	AllowN(ctx context.Context, key string, n int) (bool, error)

	// Reset clears the rate limit counter for a key
	Reset(ctx context.Context, key string) error
}

// Config represents rate limiter configuration
type Config struct {
	// Limit is the maximum number of requests allowed
	Limit int

	// Window is the time window for the limit
	Window time.Duration
}
