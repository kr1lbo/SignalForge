package pushover

import (
	"SignalForge/internal/domain/notify"
	"context"
	"fmt"
	"log/slog"
)

// Sender sends notifications via Pushover
type Sender struct {
	logger   *slog.Logger
	apiToken string

	// TODO: Add HTTP client
}

// New creates a new Pushover sender
func New(logger *slog.Logger, apiToken string) *Sender {
	return &Sender{
		logger:   logger.With("sender", "pushover"),
		apiToken: apiToken,
	}
}

// Channel returns the notification channel type
func (s *Sender) Channel() notify.Channel {
	return notify.ChannelPushover
}

// Send delivers a notification via Pushover
func (s *Sender) Send(ctx context.Context, msg notify.Message) error {
	if msg.PushoverKey == "" {
		return fmt.Errorf("pushover user key not set")
	}

	s.logger.Debug("sending pushover notification",
		"user_id", msg.UserID)

	// TODO: Send via Pushover API
	// POST https://api.pushover.net/1/messages.json
	// Body: {
	//   "token": s.apiToken,
	//   "user": msg.PushoverKey,
	//   "title": msg.Title,
	//   "message": msg.Body,
	//   "priority": msg.Priority
	// }

	s.logger.Info("pushover notification sent",
		"user_id", msg.UserID,
		"exchange", msg.Exchange,
		"symbol", msg.Symbol)

	return nil
}
