package watcher

import (
	"SignalForge/internal/domain/exchange"
	"SignalForge/internal/domain/repository"
	"SignalForge/internal/infra/symbol"
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service watches price streams and triggers alerts
type Service struct {
	logger *slog.Logger

	// Dependencies
	pool       *pgxpool.Pool              // Needed for transactions
	streams    map[string]exchange.Stream // key: exchange name
	normalizer *symbol.Normalizer         // Needed for symbol normalization
	alertRepo  repository.AlertRepository
	jobRepo    repository.NotificationJobRepository

	// Subscription tracking
	subscriptions map[subscriptionKey]*subscription
	mu            sync.RWMutex

	// Config
	debounceDelay time.Duration

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type subscriptionKey struct {
	exchange string
	symbol   string
}

type subscription struct {
	refCount      int
	unsubTimer    *time.Timer
	lastPriceTime time.Time
}

// New creates a new watcher service
func New(
	logger *slog.Logger,
	pool *pgxpool.Pool,
	streams map[string]exchange.Stream,
	normalizer *symbol.Normalizer,
	alertRepo repository.AlertRepository,
	jobRepo repository.NotificationJobRepository,
	debounceDelay time.Duration,
) *Service {
	return &Service{
		logger:        logger,
		pool:          pool,
		streams:       streams,
		normalizer:    normalizer,
		alertRepo:     alertRepo,
		jobRepo:       jobRepo,
		subscriptions: make(map[subscriptionKey]*subscription),
		debounceDelay: debounceDelay,
	}
}

// Start begins watching for price events
func (s *Service) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.logger.Info("watcher starting")

	// Start all exchange streams first
	for name, stream := range s.streams {
		s.wg.Add(1)
		go func(name string, stream exchange.Stream) {
			defer s.wg.Done()

			if err := stream.Start(s.ctx); err != nil {
				s.logger.Error("stream failed", "exchange", name, "error", err)
			}
		}(name, stream)

		// Start event processor for this stream
		s.wg.Add(1)
		go s.processEvents(stream)
	}

	// Wait a bit for streams to connect
	time.Sleep(2 * time.Second)

	// Now restore subscriptions from database
	if err := s.restoreSubscriptions(); err != nil {
		s.logger.Error("failed to restore subscriptions", "error", err)
		// Don't fail - continue anyway
	}

	s.logger.Info("watcher started")
	return nil
}

// Stop gracefully stops the watcher
func (s *Service) Stop() error {
	s.logger.Info("watcher stopping")

	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
	s.logger.Info("watcher stopped")
	return nil
}

// Subscribe adds a subscription for an exchange/symbol pair
func (s *Service) Subscribe(exchange, symbol string) error {
	key := subscriptionKey{exchange: exchange, symbol: symbol}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, exists := s.subscriptions[key]
	if !exists {
		// New subscription
		stream, ok := s.streams[exchange]
		if !ok {
			s.logger.Warn("unknown exchange", "exchange", exchange)
			return nil
		}

		// Check if stream is connected
		if !stream.IsConnected() {
			s.logger.Warn("stream not connected yet, will subscribe when connected",
				"exchange", exchange, "symbol", symbol)
			// Store subscription but don't send subscribe message yet
			s.subscriptions[key] = &subscription{refCount: 1}
			return nil
		}

		if err := stream.Subscribe(symbol); err != nil {
			return err
		}

		s.subscriptions[key] = &subscription{refCount: 1}
		s.logger.Info("subscribed", "exchange", exchange, "symbol", symbol)
	} else {
		// Existing subscription - cancel unsubscribe timer if exists
		if sub.unsubTimer != nil {
			sub.unsubTimer.Stop()
			sub.unsubTimer = nil
		}
		sub.refCount++
		s.logger.Debug("subscription ref count increased",
			"exchange", exchange, "symbol", symbol, "refCount", sub.refCount)
	}

	return nil
}

// Unsubscribe removes a subscription (with debounce)
func (s *Service) Unsubscribe(exchange, symbol string) error {
	key := subscriptionKey{exchange: exchange, symbol: symbol}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, exists := s.subscriptions[key]
	if !exists {
		return nil
	}

	sub.refCount--
	s.logger.Debug("subscription ref count decreased",
		"exchange", exchange, "symbol", symbol, "refCount", sub.refCount)

	if sub.refCount <= 0 {
		// Schedule unsubscribe with debounce
		sub.unsubTimer = time.AfterFunc(s.debounceDelay, func() {
			s.performUnsubscribe(exchange, symbol)
		})
		s.logger.Info("unsubscribe scheduled",
			"exchange", exchange, "symbol", symbol, "delay", s.debounceDelay)
	}

	return nil
}

func (s *Service) performUnsubscribe(exchange, symbol string) {
	key := subscriptionKey{exchange: exchange, symbol: symbol}

	s.mu.Lock()
	defer s.mu.Unlock()

	sub, exists := s.subscriptions[key]
	if !exists || sub.refCount > 0 {
		return // Subscription was re-added
	}

	alerts, err := s.alertRepo.FetchActiveByKey(s.ctx, exchange, symbol)
	if err != nil {
		s.logger.Error("failed to check active alerts before unsubscribe",
			"exchange", exchange,
			"symbol", symbol,
			"error", err)
		return
	}

	if len(alerts) > 0 {
		s.logger.Info("other active alerts exist, not unsubscribing",
			"exchange", exchange,
			"symbol", symbol,
			"active_alerts", len(alerts))
		sub.refCount = len(alerts)
		return
	}

	// No active alerts anywhere, safe to unsubscribe
	stream, ok := s.streams[exchange]
	if !ok {
		return
	}

	if err := stream.Unsubscribe(symbol); err != nil {
		s.logger.Error("failed to unsubscribe",
			"exchange", exchange, "symbol", symbol, "error", err)
		return
	}

	delete(s.subscriptions, key)
	s.logger.Info("unsubscribed", "exchange", exchange, "symbol", symbol)
}

func (s *Service) restoreSubscriptions() error {
	subscriptions, err := s.alertRepo.GetUniqueSubscriptions(s.ctx)
	if err != nil {
		return err
	}

	s.logger.Info("restoring subscriptions", "count", len(subscriptions))

	for _, sub := range subscriptions {
		if err := s.Subscribe(sub.Exchange, sub.Symbol); err != nil {
			s.logger.Error("failed to restore subscription",
				"exchange", sub.Exchange, "symbol", sub.Symbol, "error", err)
		}
	}

	// Schedule a retry for failed subscriptions after streams are fully connected
	time.AfterFunc(5*time.Second, func() {
		s.retryPendingSubscriptions()
	})

	return nil
}

// retryPendingSubscriptions attempts to subscribe for any subscriptions that failed initially
func (s *Service) retryPendingSubscriptions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, sub := range s.subscriptions {
		stream, ok := s.streams[key.exchange]
		if !ok {
			continue
		}

		// If stream is now connected and we haven't actually subscribed yet
		if stream.IsConnected() && sub.refCount > 0 {
			symbol := s.normalizer.Normalize(key.symbol)
			if err := stream.Subscribe(symbol); err != nil {
				s.logger.Error("retry subscribe failed",
					"exchange", key.exchange,
					"symbol", key.symbol,
					"error", err)
			} else {
				s.logger.Info("retry subscribe succeeded",
					"exchange", key.exchange,
					"symbol", key.symbol)
			}
		}
	}
}

func (s *Service) processEvents(stream exchange.Stream) {
	defer s.wg.Done()

	exchange := stream.Exchange()
	events := stream.Events()

	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				s.logger.Info("event channel closed", "exchange", exchange)
				return
			}

			if err := s.handlePriceEvent(event); err != nil {
				s.logger.Error("failed to handle price event",
					"exchange", exchange,
					"symbol", event.Symbol,
					"error", err)
			}
		}
	}
}
