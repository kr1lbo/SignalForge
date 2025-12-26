package postgres

import (
	"SignalForge/internal/domain/repository"
	"SignalForge/internal/infra/db/postgres/sqlc"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AlertRepository implements repository.AlertRepository
type AlertRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewAlertRepository creates a new AlertRepository
func NewAlertRepository(pool *pgxpool.Pool) *AlertRepository {
	return &AlertRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// Create creates a new alert
func (r *AlertRepository) Create(ctx context.Context, alert *repository.Alert) error {
	var notes pgtype.Text
	if alert.Notes != nil {
		notes = pgtype.Text{String: *alert.Notes, Valid: true}
	}

	params := sqlc.CreateAlertParams{
		UserID:    alert.UserID,
		Exchange:  alert.Exchange,
		Symbol:    alert.Symbol,
		Price:     alert.Price,
		Direction: alert.Direction,
		Notes:     notes,
	}

	dbAlert, err := r.queries.CreateAlert(ctx, params)
	if err != nil {
		return fmt.Errorf("create alert: %w", err)
	}

	// Update the alert with generated fields
	alert.ID = dbAlert.ID
	alert.IsActive = dbAlert.IsActive
	alert.CreatedAt = dbAlert.CreatedAt.Time
	alert.UpdatedAt = dbAlert.UpdatedAt.Time

	return nil
}

// GetByID retrieves an alert by ID for a specific user
func (r *AlertRepository) GetByID(ctx context.Context, alertID, userID int64) (*repository.Alert, error) {
	params := sqlc.GetAlertParams{
		ID:     alertID,
		UserID: userID,
	}

	dbAlert, err := r.queries.GetAlert(ctx, params)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("alert not found")
		}
		return nil, fmt.Errorf("get alert: %w", err)
	}

	return mapAlert(dbAlert), nil
}

// List retrieves alerts for a user with optional filtering
func (r *AlertRepository) List(ctx context.Context, userID int64, filter repository.AlertFilter) ([]*repository.Alert, error) {
	// For now, just return all alerts for user
	// TODO: Implement filtering if needed
	dbAlerts, err := r.queries.ListAllUserAlerts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}

	alerts := make([]*repository.Alert, len(dbAlerts))
	for i, dbAlert := range dbAlerts {
		alerts[i] = mapAlert(dbAlert)
	}

	return alerts, nil
}

// Delete removes an alert (only if not fired)
func (r *AlertRepository) Delete(ctx context.Context, alertID, userID int64) error {
	params := sqlc.DeleteAlertParams{
		ID:     alertID,
		UserID: userID,
	}

	if err := r.queries.DeleteAlert(ctx, params); err != nil {
		return fmt.Errorf("delete alert: %w", err)
	}

	return nil
}

// UpdateActive sets the active status of an alert
func (r *AlertRepository) UpdateActive(ctx context.Context, alertID, userID int64, isActive bool) error {
	params := sqlc.ActivateAlertParams{
		ID:     alertID,
		UserID: userID,
	}

	var err error
	if isActive {
		err = r.queries.ActivateAlert(ctx, params)
	} else {
		deactivateParams := sqlc.DeactivateAlertParams{
			ID:     alertID,
			UserID: userID,
		}
		err = r.queries.DeactivateAlert(ctx, deactivateParams)
	}

	if err != nil {
		return fmt.Errorf("update active: %w", err)
	}

	return nil
}

// MarkFired marks an alert as fired (atomic, returns error if already fired)
func (r *AlertRepository) MarkFired(ctx context.Context, alertID int64) error {
	if err := r.queries.MarkAlertFired(ctx, alertID); err != nil {
		return fmt.Errorf("mark fired: %w", err)
	}

	return nil
}

// FetchActiveByKey retrieves all active alerts for an exchange/symbol with user data
func (r *AlertRepository) FetchActiveByKey(ctx context.Context, exchange, symbol string) ([]*repository.AlertWithUser, error) {
	params := sqlc.FetchActiveAlertsByKeyParams{
		Exchange: exchange,
		Symbol:   symbol,
	}

	dbAlerts, err := r.queries.FetchActiveAlertsByKey(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("fetch active by key: %w", err)
	}

	alerts := make([]*repository.AlertWithUser, len(dbAlerts))
	for i, dbAlert := range dbAlerts {
		alerts[i] = mapAlertWithUser(dbAlert)
	}

	return alerts, nil
}

// GetUniqueSubscriptions returns all unique (exchange, symbol) pairs with active alerts
func (r *AlertRepository) GetUniqueSubscriptions(ctx context.Context) ([]repository.Subscription, error) {
	dbSubs, err := r.queries.GetUniqueActiveSubscriptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("get unique subscriptions: %w", err)
	}

	subs := make([]repository.Subscription, len(dbSubs))
	for i, dbSub := range dbSubs {
		subs[i] = repository.Subscription{
			Exchange: dbSub.Exchange,
			Symbol:   dbSub.Symbol,
		}
	}

	return subs, nil
}

// CountActive returns the number of active alerts for a user
func (r *AlertRepository) CountActive(ctx context.Context, userID int64) (int64, error) {
	count, err := r.queries.CountActiveAlerts(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count active: %w", err)
	}

	return count, nil
}

// mapAlert converts sqlc.Alert to repository.Alert
func mapAlert(dbAlert sqlc.Alert) *repository.Alert {
	var firedAt *time.Time
	if dbAlert.FiredAt.Valid {
		firedAt = &dbAlert.FiredAt.Time
	}

	var notes *string
	if dbAlert.Notes.Valid {
		notes = &dbAlert.Notes.String
	}

	return &repository.Alert{
		ID:        dbAlert.ID,
		UserID:    dbAlert.UserID,
		Exchange:  dbAlert.Exchange,
		Symbol:    dbAlert.Symbol,
		Price:     dbAlert.Price,
		Direction: dbAlert.Direction,
		Notes:     notes,
		IsActive:  dbAlert.IsActive,
		FiredAt:   firedAt,
		CreatedAt: dbAlert.CreatedAt.Time,
		UpdatedAt: dbAlert.UpdatedAt.Time,
	}
}

// mapAlertWithUser converts sqlc.FetchActiveAlertsByKeyRow to repository.AlertWithUser
func mapAlertWithUser(dbAlert sqlc.FetchActiveAlertsByKeyRow) *repository.AlertWithUser {
	var notes *string
	if dbAlert.Notes.Valid {
		notes = &dbAlert.Notes.String
	}

	var pushoverKey *string
	if dbAlert.PushoverUserKey.Valid {
		pushoverKey = &dbAlert.PushoverUserKey.String
	}

	return &repository.AlertWithUser{
		Alert: repository.Alert{
			ID:        dbAlert.ID,
			UserID:    dbAlert.UserID,
			Exchange:  dbAlert.Exchange,
			Symbol:    dbAlert.Symbol,
			Price:     dbAlert.Price,
			Direction: dbAlert.Direction,
			Notes:     notes,
			IsActive:  true, // Always true from this query
			FiredAt:   nil,  // Always nil from this query
			CreatedAt: dbAlert.CreatedAt.Time,
			UpdatedAt: time.Time{}, // Not returned by this query
		},
		TelegramID:      dbAlert.TelegramID,
		PushoverKey:     pushoverKey,
		PushoverEnabled: dbAlert.PushoverEnabled,
		TelegramEnabled: dbAlert.TelegramEnabled,
	}
}
