package symbol

import (
	"strings"
)

// Normalizer handles symbol normalization across exchanges
type Normalizer struct{}

// New creates a new symbol normalizer
func New() *Normalizer {
	return &Normalizer{}
}

// Normalize converts any symbol format to standard format (BTCUSDT)
// Examples:
//
//	BTC/USDT -> BTCUSDT
//	btc_usdt -> BTCUSDT
//	BTC-USDT -> BTCUSDT
func (n *Normalizer) Normalize(symbol string) string {
	// Remove common separators
	normalized := strings.ReplaceAll(symbol, "/", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, " ", "")

	// Convert to uppercase
	return strings.ToUpper(normalized)
}

// ToExchangeFormat converts standard format to exchange-specific format
func (n *Normalizer) ToExchangeFormat(exchange, symbol string) string {
	normalized := n.Normalize(symbol)

	switch exchange {
	case "gate":
		// Gate.io uses BTC_USDT format
		return toUnderscoreFormat(normalized)
	case "bybit":
		// Bybit uses BTCUSDT format
		return normalized
	case "binance":
		// Binance uses BTCUSDT format
		return normalized
	default:
		return normalized
	}
}

// toUnderscoreFormat converts BTCUSDT to BTC_USDT
// This is a simple heuristic: assumes common quote currencies
func toUnderscoreFormat(symbol string) string {
	// Common quote currencies (in order of priority)
	quotes := []string{"USDT", "USDC", "USD", "BTC", "ETH", "BNB"}

	for _, quote := range quotes {
		if strings.HasSuffix(symbol, quote) {
			base := strings.TrimSuffix(symbol, quote)
			return base + "_" + quote
		}
	}

	// Fallback: assume last 4 chars are quote
	if len(symbol) > 4 {
		return symbol[:len(symbol)-4] + "_" + symbol[len(symbol)-4:]
	}

	return symbol
}

// FromExchangeFormat converts exchange-specific format to standard format
func (n *Normalizer) FromExchangeFormat(exchange, symbol string) string {
	// All exchanges will be normalized to the same format
	return n.Normalize(symbol)
}
