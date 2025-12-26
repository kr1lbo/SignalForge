package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Global metrics instance (initialized in New())
var global *Metrics

// Metrics holds all application metrics
type Metrics struct {
	// Alerts
	AlertsTriggered *prometheus.CounterVec
	AlertsCreated   *prometheus.CounterVec
	AlertsDeleted   *prometheus.CounterVec
	AlertsActive    *prometheus.GaugeVec

	// Notifications
	NotificationsSent   *prometheus.CounterVec
	NotificationsFailed *prometheus.CounterVec
	NotificationLatency *prometheus.HistogramVec

	// WebSocket
	WebSocketConnected  *prometheus.GaugeVec
	WebSocketReconnects *prometheus.CounterVec
	PriceEvents         *prometheus.CounterVec

	// Jobs
	JobsProcessed *prometheus.CounterVec
	JobsFailed    *prometheus.CounterVec
	JobsRetried   *prometheus.CounterVec

	// Users
	TotalUsers      prometheus.Gauge
	ActiveUsers     prometheus.Gauge
	TelegramEnabled prometheus.Gauge
	PushoverEnabled prometheus.Gauge
}

// Get returns the global metrics instance
func Get() *Metrics {
	return global
}

// New creates and registers all metrics
func New() *Metrics {
	m := &Metrics{
		// Alert metrics
		AlertsTriggered: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_alerts_triggered_total",
				Help: "Total number of alerts that were triggered",
			},
			[]string{"exchange", "direction"},
		),
		AlertsCreated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_alerts_created_total",
				Help: "Total number of alerts created by users",
			},
			[]string{"exchange"},
		),
		AlertsDeleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_alerts_deleted_total",
				Help: "Total number of alerts deleted by users",
			},
			[]string{"exchange"},
		),
		AlertsActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "signalforge_alerts_active",
				Help: "Current number of active alerts",
			},
			[]string{"exchange"},
		),

		// Notification metrics
		NotificationsSent: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_notifications_sent_total",
				Help: "Total number of notifications successfully sent",
			},
			[]string{"channel"},
		),
		NotificationsFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_notifications_failed_total",
				Help: "Total number of notifications that failed to send",
			},
			[]string{"channel", "reason"},
		),
		NotificationLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "signalforge_notification_latency_seconds",
				Help:    "Time taken to send notification",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"channel"},
		),

		// WebSocket metrics
		WebSocketConnected: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "signalforge_websocket_connected",
				Help: "WebSocket connection status (1=connected, 0=disconnected)",
			},
			[]string{"exchange"},
		),
		WebSocketReconnects: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_websocket_reconnects_total",
				Help: "Total number of WebSocket reconnections",
			},
			[]string{"exchange"},
		),
		PriceEvents: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_price_events_total",
				Help: "Total number of price events received from exchanges",
			},
			[]string{"exchange"},
		),

		// Job metrics
		JobsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_jobs_processed_total",
				Help: "Total number of notification jobs processed",
			},
			[]string{"channel", "status"},
		),
		JobsFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_jobs_failed_total",
				Help: "Total number of notification jobs that failed permanently",
			},
			[]string{"channel"},
		),
		JobsRetried: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalforge_jobs_retried_total",
				Help: "Total number of notification jobs retried",
			},
			[]string{"channel"},
		),

		// User metrics
		TotalUsers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "signalforge_users_total",
				Help: "Total number of registered users",
			},
		),
		ActiveUsers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "signalforge_users_active",
				Help: "Number of users with at least one active alert",
			},
		),
		TelegramEnabled: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "signalforge_users_telegram_enabled",
				Help: "Number of users with Telegram notifications enabled",
			},
		),
		PushoverEnabled: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "signalforge_users_pushover_enabled",
				Help: "Number of users with Pushover notifications enabled",
			},
		),
	}

	// Set global instance
	global = m
	return m
}
