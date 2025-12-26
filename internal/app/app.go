package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"SignalForge/internal/domain/exchange"
	"SignalForge/internal/domain/notify"
	"SignalForge/internal/domain/ratelimit"
	"SignalForge/internal/infra/config"
	"SignalForge/internal/infra/db/postgres"
	"SignalForge/internal/infra/exchanges/bybit"
	"SignalForge/internal/infra/exchanges/gate"
	"SignalForge/internal/infra/metrics"
	"SignalForge/internal/infra/notification/pushover"
	"SignalForge/internal/infra/notification/telegram"
	infraRedis "SignalForge/internal/infra/redis"
	"SignalForge/internal/infra/symbol"
	"SignalForge/internal/services/notifier"
	"SignalForge/internal/services/tgbot"
	"SignalForge/internal/services/watcher"
)

// Application represents the entire SignalForge application
type Application struct {
	cfg    *config.Config
	logger *slog.Logger

	// Infrastructure
	db    *pgxpool.Pool
	redis *redis.Client

	// Metrics
	metrics       *metrics.Metrics
	metricsServer *metrics.Server

	// Services
	tgbot    *tgbot.Bot
	watcher  *watcher.Service
	notifier *notifier.Service

	wg       sync.WaitGroup
	shutdown chan struct{}
}

// New creates and initializes the application
func New(cfg *config.Config, logger *slog.Logger) (*Application, error) {
	app := &Application{
		cfg:      cfg,
		logger:   logger,
		shutdown: make(chan struct{}),
	}

	ctx := context.Background()

	// 1. Initialize database
	logger.Info("initializing database connection")
	pool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	app.db = pool
	logger.Info("database connected")

	// 2. Initialize Redis
	logger.Info("initializing redis connection")
	redisClient, err := infraRedis.NewClient(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("create redis client: %w", err)
	}
	app.redis = redisClient
	logger.Info("redis connected")

	// 3. Initialize metrics
	app.metrics = metrics.New()
	if cfg.Metrics.Enabled {
		app.metricsServer = metrics.NewServer(logger, cfg.Metrics.Port)
	}

	// 4. Create repositories
	userRepo := postgres.NewUserRepository(pool)
	alertRepo := postgres.NewAlertRepository(pool)
	jobRepo := postgres.NewJobRepository(pool)

	// 5. Create symbol normalizer
	normalizer := symbol.New()

	// 6. Create exchange streams
	streams := make(map[string]exchange.Stream)

	// Gate.io stream
	gateStream := gate.New(logger, normalizer)
	streams["gate"] = gateStream

	// Bybit stream
	bybitStream := bybit.New(logger, normalizer)
	streams["bybit"] = bybitStream

	// TODO: Add Binance when ready
	// streams["binance"] = binance.New(logger, normalizer)

	// 7. Create rate limiters
	rateLimits := make(map[notify.Channel]ratelimit.Limiter)

	// Telegram rate limiter (30 msg/min)
	tgRateLimit := infraRedis.NewLimiter(redisClient, ratelimit.Config{
		Limit:  cfg.Telegram.RateLimit,
		Window: cfg.Telegram.RetryDelay * 60, // Convert to window duration
	})
	rateLimits[notify.ChannelTelegram] = tgRateLimit

	// Pushover rate limiter (250 msg/min)
	pushoverRateLimit := infraRedis.NewLimiter(redisClient, ratelimit.Config{
		Limit:  cfg.Pushover.RateLimit,
		Window: cfg.Pushover.RetryDelay * 60,
	})
	rateLimits[notify.ChannelPushover] = pushoverRateLimit

	// 8. Create notification senders
	senders := make(map[notify.Channel]notify.Sender)

	// Telegram sender
	tgSender := telegram.New(logger, cfg.Telegram.BotToken)
	senders[notify.ChannelTelegram] = tgSender

	// Pushover sender
	pushoverSender := pushover.New(logger, cfg.Pushover.APIToken)
	senders[notify.ChannelPushover] = pushoverSender

	// 9. Create services
	logger.Info("initializing services")

	// Watcher service
	app.watcher = watcher.New(
		logger,
		pool,
		streams,
		normalizer,
		alertRepo,
		jobRepo,
		cfg.Watcher.SubscribeDebounce,
	)

	// Notifier service
	app.notifier = notifier.New(
		logger,
		jobRepo,
		senders,
		rateLimits,
		cfg.Notifier.Workers,
		cfg.Notifier.PollInterval,
		cfg.Notifier.MaxRetries,
	)

	// Telegram bot (pass watcher so it can subscribe to new alerts)
	app.tgbot = tgbot.New(
		logger,
		cfg.Telegram.BotToken,
		userRepo,
		alertRepo,
		app.watcher,
	)

	logger.Info("application initialized successfully")
	return app, nil
}

// Run starts all application components
func (a *Application) Run(ctx context.Context) error {
	a.logger.Info("application starting")

	// Start metrics server if enabled
	if a.cfg.Metrics.Enabled && a.metricsServer != nil {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.metricsServer.Start(); err != nil {
				a.logger.Error("metrics server failed", "error", err)
			}
		}()
	}

	// Start watcher (price monitoring)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.watcher.Start(ctx); err != nil {
			a.logger.Error("watcher failed", "error", err)
		}
	}()

	// Start notifier workers
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.notifier.Start(ctx); err != nil {
			a.logger.Error("notifier failed", "error", err)
		}
	}()

	// Start telegram bot
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.tgbot.Start(ctx); err != nil {
			a.logger.Error("telegram bot failed", "error", err)
		}
	}()

	a.logger.Info("all services started")

	// Block until context is cancelled
	<-ctx.Done()
	return nil
}

// Shutdown gracefully stops all components
func (a *Application) Shutdown(ctx context.Context) error {
	a.logger.Info("application shutdown initiated")
	close(a.shutdown)

	// Stop services in reverse order
	a.logger.Info("stopping telegram bot")
	if err := a.tgbot.Stop(); err != nil {
		a.logger.Error("telegram bot shutdown error", "error", err)
	}

	a.logger.Info("stopping notifier")
	if err := a.notifier.Stop(); err != nil {
		a.logger.Error("notifier shutdown error", "error", err)
	}

	a.logger.Info("stopping watcher")
	if err := a.watcher.Stop(); err != nil {
		a.logger.Error("watcher shutdown error", "error", err)
	}

	// Stop metrics server
	if a.metricsServer != nil {
		a.logger.Info("stopping metrics server")
		if err := a.metricsServer.Stop(ctx); err != nil {
			a.logger.Error("metrics server shutdown error", "error", err)
		}
	}

	// Create a done channel to signal completion
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		a.logger.Info("all services stopped")
	case <-ctx.Done():
		a.logger.Warn("shutdown timeout exceeded, forcing stop")
	}

	// Close connections
	if a.db != nil {
		a.logger.Info("closing database connection")
		a.db.Close()
	}

	if a.redis != nil {
		a.logger.Info("closing redis connection")
		if err := a.redis.Close(); err != nil {
			a.logger.Error("redis close error", "error", err)
		}
	}

	a.logger.Info("shutdown complete")
	return nil
}
