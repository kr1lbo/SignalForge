package metrics

// Helper functions for recording metrics safely (checks if metrics are initialized)

// RecordWebSocketConnected sets WebSocket connection status
func RecordWebSocketConnected(exchange string, connected bool) {
	if global == nil {
		return
	}
	value := 0.0
	if connected {
		value = 1.0
	}
	global.WebSocketConnected.WithLabelValues(exchange).Set(value)
}

// RecordWebSocketReconnect increments reconnect counter
func RecordWebSocketReconnect(exchange string) {
	if global == nil {
		return
	}
	global.WebSocketReconnects.WithLabelValues(exchange).Inc()
}

// RecordPriceEvent increments price event counter
func RecordPriceEvent(exchange string) {
	if global == nil {
		return
	}
	global.PriceEvents.WithLabelValues(exchange).Inc()
}

// RecordAlertTriggered increments triggered alerts counter
func RecordAlertTriggered(exchange, direction string) {
	if global == nil {
		return
	}
	global.AlertsTriggered.WithLabelValues(exchange, direction).Inc()
}

// RecordAlertCreated increments created alerts counter
func RecordAlertCreated(exchange string) {
	if global == nil {
		return
	}
	global.AlertsCreated.WithLabelValues(exchange).Inc()
}

// RecordAlertDeleted increments deleted alerts counter
func RecordAlertDeleted(exchange string) {
	if global == nil {
		return
	}
	global.AlertsDeleted.WithLabelValues(exchange).Inc()
}

// RecordNotificationSent increments sent notifications counter
func RecordNotificationSent(channel string) {
	if global == nil {
		return
	}
	global.NotificationsSent.WithLabelValues(channel).Inc()
}

// RecordNotificationFailed increments failed notifications counter
func RecordNotificationFailed(channel, reason string) {
	if global == nil {
		return
	}
	global.NotificationsFailed.WithLabelValues(channel, reason).Inc()
}

// SetTotalUsers sets total users gauge
func SetTotalUsers(count float64) {
	if global == nil {
		return
	}
	global.TotalUsers.Set(count)
}
