package watcher

import (
	"SignalForge/internal/domain/notify"
	"SignalForge/internal/domain/price"
	"SignalForge/internal/domain/repository"
	"SignalForge/internal/infra/db/postgres"
	"SignalForge/internal/infra/db/postgres/sqlc"
	"encoding/json"
	"fmt"
	"time"
)

// handlePriceEvent processes a price event and triggers alerts
func (s *Service) handlePriceEvent(event price.Event) error {
	// Fetch all active alerts for this exchange/symbol
	alerts, err := s.alertRepo.FetchActiveByKey(s.ctx, event.Exchange, event.Symbol)
	if err != nil {
		return fmt.Errorf("fetch active alerts: %w", err)
	}

	if len(alerts) == 0 {
		return nil // No alerts to check
	}

	s.logger.Debug("checking alerts",
		"exchange", event.Exchange,
		"symbol", event.Symbol,
		"price", event.MarkPrice,
		"alert_count", len(alerts))

	// Check each alert
	for _, alert := range alerts {
		if s.shouldTrigger(alert, event.MarkPrice) {
			if err := s.triggerAlert(alert, event); err != nil {
				s.logger.Error("failed to trigger alert",
					"alert_id", alert.ID,
					"user_id", alert.UserID,
					"error", err)
				// Continue processing other alerts
				continue
			}

			s.logger.Info("alert triggered",
				"alert_id", alert.ID,
				"user_id", alert.UserID,
				"exchange", event.Exchange,
				"symbol", event.Symbol,
				"target_price", alert.Price,
				"actual_price", event.MarkPrice,
				"direction", alert.Direction)
		}
	}

	return nil
}

// shouldTrigger checks if an alert should trigger based on price
func (s *Service) shouldTrigger(alert *repository.AlertWithUser, currentPrice float64) bool {
	direction := price.Direction(alert.Direction)
	return direction.ShouldTrigger(alert.Price, currentPrice)
}

// triggerAlert marks alert as fired and creates notification jobs in a transaction
func (s *Service) triggerAlert(alert *repository.AlertWithUser, event price.Event) error {
	// Get transaction-aware queries
	tx, queries, err := postgres.GetTx(s.ctx, s.pool)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(s.ctx)

	// 1. Mark alert as fired
	if err := queries.MarkAlertFired(s.ctx, alert.ID); err != nil {
		return fmt.Errorf("mark alert fired: %w", err)
	}

	// 2. Create notification jobs
	jobs := s.createNotificationJobs(alert, event)

	for _, job := range jobs {
		payload, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("marshal job payload: %w", err)
		}

		params := sqlc.InsertNotificationJobParams{
			AlertID: alert.ID,
			UserID:  alert.UserID,
			Channel: string(job.Channel),
			Payload: payload,
		}

		if err := queries.InsertNotificationJob(s.ctx, params); err != nil {
			return fmt.Errorf("insert notification job: %w", err)
		}
	}

	// 3. Commit transaction
	if err := tx.Commit(s.ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// NotificationPayload represents the data stored in notification_jobs.payload
type NotificationPayload struct {
	Channel     notify.Channel `json:"channel"`
	UserID      int64          `json:"user_id"`
	TelegramID  int64          `json:"telegram_id,omitempty"`
	PushoverKey string         `json:"pushover_key,omitempty"`

	// Message data
	Title    string `json:"title"`
	Body     string `json:"body"`
	Priority int    `json:"priority"`

	// Alert context
	Exchange  string    `json:"exchange"`
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Direction string    `json:"direction"`
	Notes     string    `json:"notes,omitempty"`
	FiredAt   time.Time `json:"fired_at"`
}

// createNotificationJobs creates notification job payloads for an alert
func (s *Service) createNotificationJobs(alert *repository.AlertWithUser, event price.Event) []NotificationPayload {
	var jobs []NotificationPayload

	now := time.Now()
	notes := ""
	if alert.Notes != nil {
		notes = *alert.Notes
	}

	// Format message body
	body := fmt.Sprintf(
		"🔔 Alert Triggered!\n\n"+
			"Exchange: %s\n"+
			"Symbol: %s\n"+
			"Target Price: $%.2f\n"+
			"Current Price: $%.2f\n"+
			"Direction: %s",
		event.Exchange,
		event.Symbol,
		alert.Price,
		event.MarkPrice,
		alert.Direction,
	)

	if notes != "" {
		body += fmt.Sprintf("\nNotes: %s", notes)
	}

	// Telegram notification (if enabled)
	if alert.TelegramEnabled {
		jobs = append(jobs, NotificationPayload{
			Channel:    notify.ChannelTelegram,
			UserID:     alert.UserID,
			TelegramID: alert.TelegramID,
			Title:      "Price Alert",
			Body:       body,
			Priority:   0,
			Exchange:   event.Exchange,
			Symbol:     event.Symbol,
			Price:      event.MarkPrice,
			Direction:  alert.Direction,
			Notes:      notes,
			FiredAt:    now,
		})
	}

	// Pushover notification (if enabled and configured)
	if alert.PushoverEnabled && alert.PushoverKey != nil {
		jobs = append(jobs, NotificationPayload{
			Channel:     notify.ChannelPushover,
			UserID:      alert.UserID,
			PushoverKey: *alert.PushoverKey,
			Title:       "Price Alert",
			Body:        body,
			Priority:    1, // High priority for Pushover
			Exchange:    event.Exchange,
			Symbol:      event.Symbol,
			Price:       event.MarkPrice,
			Direction:   alert.Direction,
			Notes:       notes,
			FiredAt:     now,
		})
	}

	return jobs
}
