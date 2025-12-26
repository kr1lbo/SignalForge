package pushover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"SignalForge/internal/domain/notify"
)

const pushoverAPIURL = "https://api.pushover.net/1/messages.json"

// Sender sends notifications via Pushover
type Sender struct {
	logger   *slog.Logger
	apiToken string // Your application API token
	client   *http.Client
}

// New creates a new Pushover sender
func New(logger *slog.Logger, apiToken string) *Sender {
	return &Sender{
		logger:   logger.With("sender", "pushover"),
		apiToken: apiToken,
		client:   &http.Client{},
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
		"user_id", msg.UserID,
		"pushover_key", msg.PushoverKey)

	// Prepare request (form-encoded)
	data := url.Values{
		"token":   {s.apiToken},
		"user":    {msg.PushoverKey},
		"title":   {msg.Title},
		"message": {s.formatMessage(msg)},
	}

	// Set priority (Pushover: -2 to 2, where 1 = high priority)
	if msg.Priority > 0 {
		data.Set("priority", "1")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pushoverAPIURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result struct {
		Status  int      `json:"status"`
		Request string   `json:"request"`
		Errors  []string `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	s.logger.Debug("pushover api response",
		"status", result.Status,
		"request", result.Request,
		"errors", result.Errors,
		"http_status", resp.StatusCode)

	if result.Status != 1 {
		if len(result.Errors) > 0 {
			return fmt.Errorf("pushover api error: %v", result.Errors)
		}
		return fmt.Errorf("pushover api returned status %d (expected 1)", result.Status)
	}

	s.logger.Info("pushover notification sent",
		"pushover_key", msg.PushoverKey,
		"exchange", msg.Exchange,
		"symbol", msg.Symbol,
		"request_id", result.Request)

	return nil
}

func (s *Sender) formatMessage(msg notify.Message) string {
	// Plain text format (Pushover doesn't support Markdown)
	priceFormat := s.formatPrice(msg.Price)

	text := fmt.Sprintf(
		"Exchange: %s\n"+
			"Symbol: %s\n"+
			"Target Price: %s\n"+
			"Direction: %s",
		msg.Exchange,
		msg.Symbol,
		priceFormat,
		msg.Direction,
	)

	if msg.Notes != "" {
		text += fmt.Sprintf("\nNotes: %s", msg.Notes)
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
