package repository

import (
	"context"
	"time"
)

// AlertFilter represents criteria for querying alerts
type AlertFilter struct {
	Exchange string
	Symbol   string
	IsActive *bool
	IsFired  *bool
}

// Alert represents the domain alert model
type Alert struct {
	ID        int64
	UserID    int64
	Exchange  string
	Symbol    string
	Price     float64
	Direction string
	Notes     *string
	IsActive  bool
	FiredAt   *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AlertWithUser extends Alert with user notification settings
type AlertWithUser struct {
	Alert
	TelegramID      int64
	PushoverKey     *string
	PushoverEnabled bool
	TelegramEnabled bool
}

// AlertRepository handles alert persistence
type AlertRepository interface {
	// Create creates a new alert
	Create(ctx context.Context, alert *Alert) error

	// GetByID retrieves an alert by ID for a specific user
	GetByID(ctx context.Context, alertID, userID int64) (*Alert, error)

	// List retrieves alerts for a user with optional filtering
	List(ctx context.Context, userID int64, filter AlertFilter) ([]*Alert, error)

	// Delete removes an alert (only if not fired)
	Delete(ctx context.Context, alertID, userID int64) error

	// UpdateActive sets the active status of an alert
	UpdateActive(ctx context.Context, alertID, userID int64, isActive bool) error

	// MarkFired marks an alert as fired (atomic, returns error if already fired)
	MarkFired(ctx context.Context, alertID int64) error

	// FetchActiveByKey retrieves all active alerts for an exchange/symbol with user data
	FetchActiveByKey(ctx context.Context, exchange, symbol string) ([]*AlertWithUser, error)

	// GetUniqueSubscriptions returns all unique (exchange, symbol) pairs with active alerts
	GetUniqueSubscriptions(ctx context.Context) ([]Subscription, error)

	// CountActive returns the number of active alerts for a user
	CountActive(ctx context.Context, userID int64) (int64, error)
}

// Subscription represents a unique exchange/symbol pair
type Subscription struct {
	Exchange string
	Symbol   string
}
