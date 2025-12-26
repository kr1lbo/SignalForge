package notify

import (
	"context"
	"time"
)

// Channel represents a notification delivery method
type Channel string

const (
	ChannelTelegram Channel = "telegram"
	ChannelPushover Channel = "pushover"
)

// Message represents a notification to be sent
type Message struct {
	UserID      int64
	TelegramID  int64  // for telegram channel
	PushoverKey string // for pushover channel

	Title    string
	Body     string
	Priority int // for pushover: -2 to 2

	// Context for formatting
	Exchange  string
	Symbol    string
	Price     float64
	Direction string
	Notes     string
	FiredAt   time.Time
}

// Sender is responsible for delivering notifications via a specific channel
type Sender interface {
	// Channel returns the channel type this sender handles
	Channel() Channel

	// Send delivers a message
	// Returns error if delivery failed
	Send(ctx context.Context, msg Message) error
}
