package telegram

import (
	"SignalForge/internal/domain/notify"
	"context"
	"fmt"
	"log/slog"
)

// Sender sends notifications via Telegram
type Sender struct {
	logger *slog.Logger
	token  string

	// TODO: Add telegram bot API client
}

// New creates a new Telegram sender
func New(logger *slog.Logger, token string) *Sender {
	return &Sender{
		logger: logger.With("sender", "telegram"),
		token:  token,
	}
}

// Channel returns the notification channel type
func (s *Sender) Channel() notify.Channel {
	return notify.ChannelTelegram
}

// Send delivers a notification via Telegram
func (s *Sender) Send(ctx context.Context, msg notify.Message) error {
	s.logger.Debug("sending telegram notification",
		"user_id", msg.UserID,
		"telegram_id", msg.TelegramID)

	// TODO: Format message
	text := s.formatMessage(msg)

	// TODO: Send via Telegram Bot API
	// Use sendMessage API with chat_id = msg.TelegramID

	s.logger.Info("telegram notification sent",
		"telegram_id", msg.TelegramID,
		"exchange", msg.Exchange,
		"symbol", msg.Symbol)

	return nil
}

func (s *Sender) formatMessage(msg notify.Message) string {
	// Format:
	// 🔔 Alert Triggered!
	//
	// Exchange: Gate.io
	// Symbol: BTC/USDT
	// Price: $42,150.50
	// Direction: Above
	// Notes: [if present]

	return fmt.Sprintf(
		"🔔 *Alert Triggered!*\n\n"+
			"*Exchange:* %s\n"+
			"*Symbol:* %s\n"+
			"*Price:* $%.2f\n"+
			"*Direction:* %s\n"+
			"%s",
		msg.Exchange,
		msg.Symbol,
		msg.Price,
		msg.Direction,
		formatNotes(msg.Notes),
	)
}

func formatNotes(notes string) string {
	if notes == "" {
		return ""
	}
	return fmt.Sprintf("*Notes:* %s\n", notes)
}
