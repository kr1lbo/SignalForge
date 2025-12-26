package tgbot

import (
	"SignalForge/internal/domain/repository"
	"context"
	"log/slog"
)

// Bot represents the Telegram bot service
type Bot struct {
	logger *slog.Logger
	token  string

	// Repositories
	userRepo  repository.UserRepository
	alertRepo repository.AlertRepository

	// TODO: Add telegram bot API client
	// TODO: Add command handlers
	// TODO: Add wizard for alert creation
}

// New creates a new Telegram bot
func New(
	logger *slog.Logger,
	token string,
	userRepo repository.UserRepository,
	alertRepo repository.AlertRepository,
) *Bot {
	return &Bot{
		logger:    logger.With("service", "tgbot"),
		token:     token,
		userRepo:  userRepo,
		alertRepo: alertRepo,
	}
}

// Start begins processing Telegram updates
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("telegram bot starting")

	// TODO: Initialize telegram bot API client
	// TODO: Start long polling or webhook
	// TODO: Register command handlers:
	//   /start - welcome message
	//   /new - create new alert (wizard)
	//   /list - list active alerts
	//   /delete - delete alert
	//   /settings - configure notifications

	<-ctx.Done()
	return nil
}

// Stop gracefully stops the bot
func (b *Bot) Stop() error {
	b.logger.Info("telegram bot stopping")

	// TODO: Stop polling/webhook
	// TODO: Close connections

	return nil
}
