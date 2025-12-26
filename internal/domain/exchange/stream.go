package exchange

import (
	"SignalForge/internal/domain/price"
	"context"
)

// Stream represents a WebSocket connection to an exchange
// It manages subscriptions and emits price events
type Stream interface {
	// Exchange returns the exchange identifier (e.g., "gate", "bybit")
	Exchange() string

	// Start begins the WebSocket connection and event processing
	Start(ctx context.Context) error

	// Subscribe adds a symbol to the subscription list
	// Should be idempotent - calling multiple times for same symbol is safe
	Subscribe(symbol string) error

	// Unsubscribe removes a symbol from the subscription list
	// Should be idempotent - calling multiple times for same symbol is safe
	Unsubscribe(symbol string) error

	// Events returns a channel that emits price updates
	// The channel is closed when the stream is stopped
	Events() <-chan price.Event

	// IsConnected returns true if the WebSocket connection is active
	IsConnected() bool
}

// SymbolNormalizer normalizes trading pair symbols to a standard format
type SymbolNormalizer interface {
	// Normalize converts a symbol to the standard format (e.g., BTCUSDT)
	Normalize(symbol string) string

	// ToExchangeFormat converts from standard format to exchange-specific format
	ToExchangeFormat(exchange, symbol string) string
}
