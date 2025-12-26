package price

import "time"

// Event represents a price update from an exchange
type Event struct {
	Exchange  string
	Symbol    string
	MarkPrice float64
	Timestamp time.Time
}

// Direction represents the alert direction
type Direction string

const (
	DirectionAbove Direction = "above"
	DirectionBelow Direction = "below"
)

// ShouldTrigger checks if the current price should trigger an alert
func (d Direction) ShouldTrigger(alertPrice, currentPrice float64) bool {
	switch d {
	case DirectionAbove:
		return currentPrice >= alertPrice
	case DirectionBelow:
		return currentPrice <= alertPrice
	default:
		return false
	}
}
