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
	// Format price with appropriate precision
	priceFormat := s.formatPrice(msg.Price)

	text := fmt.Sprintf(
		"🔔 *Alert Triggered!*\n\n"+
			"*Exchange:* %s\n"+
			"*Symbol:* %s\n"+
			"*Target Price:* %s\n"+
			"*Direction:* %s",
		msg.Exchange,
		msg.Symbol,
		priceFormat,
		msg.Direction,
	)

	if msg.Notes != "" {
		text += fmt.Sprintf("\n*Notes:* %s", msg.Notes)
	}

	return text
}

// formatPrice formats price with appropriate precision based on magnitude
func (s *Sender) formatPrice(price float64) string {
	switch {
	case price >= 1000:
		// Large prices: $42,150.50
		return fmt.Sprintf("$%,.2f", price)
	case price >= 1:
		// Medium prices: $42.15
		return fmt.Sprintf("$%.2f", price)
	case price >= 0.01:
		// Small prices: $0.0785
		return fmt.Sprintf("$%.4f", price)
	case price >= 0.0001:
		// Very small prices: $0.000785
		return fmt.Sprintf("$%.6f", price)
	default:
		// Tiny prices: $0.00000785
		return fmt.Sprintf("$%.8f", price)
	}
}
