package redis

import (
	"SignalForge/internal/domain/ratelimit"
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter implements rate limiting using Redis
type Limiter struct {
	client *redis.Client
	config ratelimit.Config
}

// NewLimiter creates a new Redis-based rate limiter
func NewLimiter(client *redis.Client, config ratelimit.Config) *Limiter {
	return &Limiter{
		client: client,
		config: config,
	}
}

// Allow checks if an action is allowed under the rate limit
func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN checks if N actions are allowed under the rate limit
func (l *Limiter) AllowN(ctx context.Context, key string, n int) (bool, error) {
	// Use sliding window algorithm with sorted sets
	now := time.Now()
	windowStart := now.Add(-l.config.Window)

	pipe := l.client.Pipeline()

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count current entries
	countCmd := pipe.ZCard(ctx, key)

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("pipeline exec: %w", err)
	}

	currentCount := countCmd.Val()

	// Check if we can allow this request
	if int(currentCount)+n > l.config.Limit {
		return false, nil
	}

	// Add new entries
	members := make([]redis.Z, n)
	for i := 0; i < n; i++ {
		members[i] = redis.Z{
			Score:  float64(now.Add(time.Duration(i) * time.Millisecond).UnixNano()),
			Member: fmt.Sprintf("%d_%d", now.UnixNano(), i),
		}
	}

	if err := l.client.ZAdd(ctx, key, members...).Err(); err != nil {
		return false, fmt.Errorf("zadd: %w", err)
	}

	// Set expiration
	if err := l.client.Expire(ctx, key, l.config.Window).Err(); err != nil {
		return false, fmt.Errorf("expire: %w", err)
	}

	return true, nil
}

// Reset clears the rate limit counter for a key
func (l *Limiter) Reset(ctx context.Context, key string) error {
	return l.client.Del(ctx, key).Err()
}
