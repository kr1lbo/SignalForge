package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"SignalForge/internal/domain/price"
	"SignalForge/internal/infra/metrics"
	"SignalForge/internal/infra/symbol"
)

const (
	gateWSURL = "wss://fx-ws.gateio.ws/v4/ws/usdt"

	// Gate.io WebSocket limits
	maxSubscriptionsPerConn = 100
	pingInterval            = 30 * time.Second
	reconnectDelay          = 5 * time.Second
)

// Stream implements exchange.Stream for Gate.io
type Stream struct {
	logger     *slog.Logger
	normalizer *symbol.Normalizer

	// WebSocket connection
	conn   *websocket.Conn
	connMu sync.RWMutex

	// Subscriptions
	subscriptions map[string]bool // symbol -> subscribed
	subMu         sync.RWMutex

	// Events channel
	events chan price.Event

	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	reconnectC chan struct{}
}

// New creates a new Gate.io WebSocket stream
func New(logger *slog.Logger, normalizer *symbol.Normalizer) *Stream {
	return &Stream{
		logger:        logger.With("exchange", "gate"),
		normalizer:    normalizer,
		subscriptions: make(map[string]bool),
		events:        make(chan price.Event, 100),
		reconnectC:    make(chan struct{}, 1),
	}
}

// Exchange returns the exchange identifier
func (s *Stream) Exchange() string {
	return "gate"
}

// Start begins the WebSocket connection
func (s *Stream) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.logger.Info("starting gate.io stream")

	// Connect
	if err := s.connect(); err != nil {
		return fmt.Errorf("initial connection failed: %w", err)
	}

	// Start ping/pong handler
	s.wg.Add(1)
	go s.pingHandler()

	// Start message reader
	s.wg.Add(1)
	go s.readMessages()

	// Start reconnect handler
	s.wg.Add(1)
	go s.reconnectHandler()

	s.logger.Info("gate.io stream started")

	<-s.ctx.Done()
	s.cleanup()
	s.wg.Wait()

	return nil
}

// Subscribe adds a symbol to the subscription list
func (s *Stream) Subscribe(symbol string) error {
	normalized := s.normalizer.Normalize(symbol)
	gateSymbol := s.normalizer.ToExchangeFormat("gate", normalized)

	s.subMu.Lock()
	defer s.subMu.Unlock()

	if s.subscriptions[normalized] {
		return nil // Already subscribed
	}

	// Send subscribe message
	msg := subscribeMessage{
		Time:    time.Now().Unix(),
		Channel: "futures.tickers",
		Event:   "subscribe",
		Payload: []string{gateSymbol},
	}

	if err := s.sendMessage(msg); err != nil {
		return fmt.Errorf("send subscribe: %w", err)
	}

	s.subscriptions[normalized] = true
	s.logger.Info("subscribed", "symbol", normalized, "gate_symbol", gateSymbol)

	return nil
}

// Unsubscribe removes a symbol from the subscription list
func (s *Stream) Unsubscribe(symbol string) error {
	normalized := s.normalizer.Normalize(symbol)
	gateSymbol := s.normalizer.ToExchangeFormat("gate", normalized)

	s.subMu.Lock()
	defer s.subMu.Unlock()

	if !s.subscriptions[normalized] {
		return nil // Not subscribed
	}

	// Send unsubscribe message
	msg := subscribeMessage{
		Time:    time.Now().Unix(),
		Channel: "futures.tickers",
		Event:   "unsubscribe",
		Payload: []string{gateSymbol},
	}

	if err := s.sendMessage(msg); err != nil {
		return fmt.Errorf("send unsubscribe: %w", err)
	}

	delete(s.subscriptions, normalized)
	s.logger.Info("unsubscribed", "symbol", normalized)

	return nil
}

// Events returns the events channel
func (s *Stream) Events() <-chan price.Event {
	return s.events
}

// IsConnected returns connection status
func (s *Stream) IsConnected() bool {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.conn != nil
}

func (s *Stream) connect() error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	s.logger.Info("connecting to gate.io websocket")

	conn, _, err := websocket.DefaultDialer.Dial(gateWSURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	s.conn = conn
	s.logger.Info("connected to gate.io websocket")

	// Record WebSocket connected metric
	metrics.RecordWebSocketConnected("gate", true)

	// Resubscribe to all symbols
	s.subMu.RLock()
	symbols := make([]string, 0, len(s.subscriptions))
	for symbol := range s.subscriptions {
		symbols = append(symbols, symbol)
	}
	s.subMu.RUnlock()

	for _, symbol := range symbols {
		gateSymbol := s.normalizer.ToExchangeFormat("gate", symbol)
		msg := subscribeMessage{
			Time:    time.Now().Unix(),
			Channel: "futures.tickers",
			Event:   "subscribe",
			Payload: []string{gateSymbol},
		}

		if err := s.sendMessageLocked(msg); err != nil {
			s.logger.Error("failed to resubscribe", "symbol", symbol, "error", err)
		}
	}

	return nil
}

func (s *Stream) sendMessage(msg interface{}) error {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.sendMessageLocked(msg)
}

func (s *Stream) sendMessageLocked(msg interface{}) error {
	if s.conn == nil {
		return fmt.Errorf("not connected")
	}

	return s.conn.WriteJSON(msg)
}

func (s *Stream) readMessages() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		s.connMu.RLock()
		conn := s.conn
		s.connMu.RUnlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			s.logger.Error("read error", "error", err)
			s.triggerReconnect()
			// Wait a bit before trying to read again to avoid tight loop
			time.Sleep(1 * time.Second)
			continue
		}

		if err := s.handleMessage(data); err != nil {
			s.logger.Error("handle message error", "error", err)
		}
	}
}

func (s *Stream) handleMessage(data []byte) error {
	var base baseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return fmt.Errorf("unmarshal base: %w", err)
	}

	// Handle different message types
	switch base.Event {
	case "update":
		if base.Channel == "futures.tickers" {
			return s.handleTickerUpdate(data)
		}
	case "subscribe", "unsubscribe":
		s.logger.Debug("subscription response", "event", base.Event, "channel", base.Channel)
	default:
		s.logger.Debug("unknown event", "event", base.Event)
	}

	return nil
}

func (s *Stream) handleTickerUpdate(data []byte) error {
	var msg tickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("unmarshal ticker: %w", err)
	}

	if len(msg.Result) == 0 {
		return nil
	}

	ticker := msg.Result[0]

	// Parse mark price
	markPrice := 0.0
	if _, err := fmt.Sscanf(ticker.MarkPrice, "%f", &markPrice); err != nil {
		return fmt.Errorf("parse mark price: %w", err)
	}

	// Normalize symbol
	normalized := s.normalizer.FromExchangeFormat("gate", ticker.Contract)

	event := price.Event{
		Exchange:  "gate",
		Symbol:    normalized,
		MarkPrice: markPrice,
		Timestamp: time.Now(),
	}

	select {
	case s.events <- event:
	case <-s.ctx.Done():
	default:
		s.logger.Warn("events channel full, dropping event", "symbol", normalized)
	}

	return nil
}

func (s *Stream) pingHandler() {
	defer s.wg.Done()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			msg := pingMessage{
				Time:    time.Now().Unix(),
				Channel: "futures.ping",
			}

			if err := s.sendMessage(msg); err != nil {
				s.logger.Error("ping failed", "error", err)
				s.triggerReconnect()
			}
		}
	}
}

func (s *Stream) reconnectHandler() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.reconnectC:
			// Close old connection first
			s.connMu.Lock()
			if s.conn != nil {
				s.conn.Close()
				s.conn = nil
			}
			s.connMu.Unlock()

			s.logger.Info("reconnecting...")

			// Record reconnect attempt
			metrics.RecordWebSocketReconnect("gate")

			// Wait before reconnecting
			time.Sleep(reconnectDelay)

			// Try to reconnect
			if err := s.connect(); err != nil {
				s.logger.Error("reconnect failed", "error", err)
				// Retry after delay
				time.AfterFunc(reconnectDelay, func() {
					select {
					case s.reconnectC <- struct{}{}:
					default:
					}
				})
			} else {
				s.logger.Info("reconnected successfully")
			}
		}
	}
}

func (s *Stream) triggerReconnect() {
	select {
	case s.reconnectC <- struct{}{}:
	default:
		// Already triggered
	}
}

func (s *Stream) cleanup() {
	s.logger.Info("cleaning up gate.io stream")

	if s.cancel != nil {
		s.cancel()
	}

	s.connMu.Lock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.connMu.Unlock()

	close(s.events)
}

// Message types
type baseMessage struct {
	Time    int64  `json:"time"`
	Channel string `json:"channel"`
	Event   string `json:"event"`
}

type subscribeMessage struct {
	Time    int64    `json:"time"`
	Channel string   `json:"channel"`
	Event   string   `json:"event"`
	Payload []string `json:"payload,omitempty"`
}

type pingMessage struct {
	Time    int64  `json:"time"`
	Channel string `json:"channel"`
}

type tickerMessage struct {
	Time    int64        `json:"time"`
	Channel string       `json:"channel"`
	Event   string       `json:"event"`
	Result  []tickerData `json:"result"`
}

type tickerData struct {
	Contract   string `json:"contract"`
	MarkPrice  string `json:"mark_price"`
	IndexPrice string `json:"index_price"`
	Last       string `json:"last"`
}
