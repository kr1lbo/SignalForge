package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"SignalForge/internal/domain/notify"
)

const telegramAPIURL = "https://api.telegram.org/bot%s/sendMessage"

// Sender sends notifications via Telegram
type Sender struct {
	logger *slog.Logger
	token  string
	client *http.Client
}

// New creates a new Telegram sender
func New(logger *slog.Logger, token string) *Sender {
	return &Sender{
		logger: logger.With("sender", "telegram"),
		token:  token,
		client: &http.Client{},
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

	// Format message
	text := s.formatMessage(msg)

	// Prepare request
	url := fmt.Sprintf(telegramAPIURL, s.token)

	payload := map[string]interface{}{
		"chat_id":    msg.TelegramID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Description string `json:"description"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("telegram api error: %s (status: %d)", errResp.Description, resp.StatusCode)
	}

	s.logger.Info("telegram notification sent",
		"telegram_id", msg.TelegramID,
		"exchange", msg.Exchange,
		"symbol", msg.Symbol)

	return nil
}

func (s *Sender) formatMessage(msg notify.Message) string {
	// Format:
	// 🔔 *Alert Triggered!*
	//
	// *Exchange:* Gate.io
	// *Symbol:* BTC/USDT
	// *Target Price:* $42,150.50
	// *Current Price:* $42,200.00
	// *Direction:* Above
	// *Notes:* [if present]

	text := fmt.Sprintf(
		"🔔 *Alert Triggered!*\n\n"+
			"*Exchange:* %s\n"+
			"*Symbol:* %s\n"+
			"*Target Price:* $%.2f\n"+
			"*Current Price:* $%.2f\n"+
			"*Direction:* %s",
		msg.Exchange,
		msg.Symbol,
		msg.Price, // This is actually target price from alert
		msg.Price, // Current price (we should fix this in watcher)
		msg.Direction,
	)

	if msg.Notes != "" {
		text += fmt.Sprintf("\n*Notes:* %s", msg.Notes)
	}

	return text
}
