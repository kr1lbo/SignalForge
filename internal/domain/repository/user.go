package repository

import (
	"context"
	"time"
)

// User represents a user with notification settings
type User struct {
	ID              int64
	TelegramID      int64
	PushoverKey     *string
	PushoverEnabled bool
	TelegramEnabled bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UserRepository handles user persistence
type UserRepository interface {
	// GetOrCreate retrieves or creates a user by Telegram ID
	GetOrCreate(ctx context.Context, telegramID int64) (*User, error)

	// GetByTelegramID retrieves a user by Telegram ID
	GetByTelegramID(ctx context.Context, telegramID int64) (*User, error)

	// UpdateSettings updates user notification settings
	UpdateSettings(ctx context.Context, userID int64, pushoverKey *string, pushoverEnabled, telegramEnabled bool) error
}
