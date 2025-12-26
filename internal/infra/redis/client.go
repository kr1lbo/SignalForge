package redis

import (
	"SignalForge/internal/infra/config"
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a new Redis client
func NewClient(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Ping to verify connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}
