package postgres

import (
	"SignalForge/internal/domain/repository"
	"SignalForge/internal/infra/db/postgres/sqlc"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository implements repository.UserRepository
type UserRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// GetOrCreate retrieves or creates a user by Telegram ID
func (r *UserRepository) GetOrCreate(ctx context.Context, telegramID int64) (*repository.User, error) {
	dbUser, err := r.queries.GetOrCreateUser(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("get or create user: %w", err)
	}

	return mapUser(dbUser), nil
}

// GetByTelegramID retrieves a user by Telegram ID
func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*repository.User, error) {
	dbUser, err := r.queries.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("get user by telegram id: %w", err)
	}

	return mapUser(dbUser), nil
}

// UpdateSettings updates user notification settings
func (r *UserRepository) UpdateSettings(ctx context.Context, userID int64, pushoverKey *string, pushoverEnabled, telegramEnabled bool) error {
	var pushoverKeyVal pgtype.Text
	if pushoverKey != nil {
		pushoverKeyVal = pgtype.Text{String: *pushoverKey, Valid: true}
	}

	params := sqlc.UpdateUserSettingsParams{
		ID:              userID,
		PushoverUserKey: pushoverKeyVal,
		PushoverEnabled: pushoverEnabled,
		TelegramEnabled: telegramEnabled,
	}

	if err := r.queries.UpdateUserSettings(ctx, params); err != nil {
		return fmt.Errorf("update user settings: %w", err)
	}

	return nil
}

// mapUser converts sqlc.User to repository.User
func mapUser(dbUser sqlc.User) *repository.User {
	var pushoverKey *string
	if dbUser.PushoverUserKey.Valid {
		pushoverKey = &dbUser.PushoverUserKey.String
	}

	return &repository.User{
		ID:              dbUser.ID,
		TelegramID:      dbUser.TelegramID,
		PushoverKey:     pushoverKey,
		PushoverEnabled: dbUser.PushoverEnabled,
		TelegramEnabled: dbUser.TelegramEnabled,
		CreatedAt:       dbUser.CreatedAt.Time,
		UpdatedAt:       dbUser.UpdatedAt.Time,
	}
}
