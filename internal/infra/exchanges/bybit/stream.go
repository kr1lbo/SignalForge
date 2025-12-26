package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"SignalForge/internal/domain/exchange"
	"SignalForge/internal/domain/price"
	"SignalForge/internal/infra/symbol"
)

const (
	bybitWSURL     = "wss://stream.bybit.com/v5/public/linear"
	reconnectDelay = 5 * time.Second
)

// Stream implements Bybit WebSocket connection
type Stream struct {
	logger     *slog.Logger
	normalizer *symbol.Normalizer

	// Connection management
	conn   *websocket.Conn
	connMu sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Subscription management
	subscriptions map[string]bool // normalized symbol -> subscribed
	subMu         sync.Mutex

	// Events channel
	events chan price.Event

	// Reconnection
	reconnectC chan struct{}
}

// Message types
type subscribeMessage struct {
	Op   string   `json:"op"`
	Args []string `json:"args"`
}

type tickerMessage struct {
	Topic string     `json:"topic"`
	Type  string     `json:"type"`
	Data  tickerData `json:"data"`
	Ts    int64      `json:"ts"`
}

type tickerData struct {
	Symbol       string `json:"symbol"`
	LastPrice    string `json:"lastPrice"`
	MarkPrice    string `json:"markPrice"`
	IndexPrice   string `json:"indexPrice"`
	PrevPrice24h string `json:"prevPrice24h"`
	Price24hPcnt string `json:"price24hPcnt"`
	HighPrice24h string `json:"highPrice24h"`
	LowPrice24h  string `json:"lowPrice24h"`
	Volume24h    string `json:"volume24h"`
	Turnover24h  string `json:"turnover24h"`
}

type pingMessage struct {
	Op    string `json:"op"`
	ReqId string `json:"req_id,omitempty"`
}

type pongMessage struct {
	Success bool   `json:"success"`
	RetMsg  string `json:"ret_msg"`
	ConnId  string `json:"conn_id"`
	Op      string `json:"op"`
}

// New creates a new Bybit stream
func New(logger *slog.Logger, normalizer *symbol.Normalizer) *Stream {
	return &Stream{
		logger:        logger.With("exchange", "bybit"),
		normalizer:    normalizer,
		subscriptions: make(map[string]bool),
		events:        make(chan price.Event, 100),
		reconnectC:    make(chan struct{}, 1),
	}
}

// Start begins the WebSocket connection
func (s *Stream) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.logger.Info("starting bybit stream")

	// Initial connection
	if err := s.connect(); err != nil {
		return fmt.Errorf("initial connection: %w", err)
	}

	// Start reconnect handler
	s.wg.Add(1)
	go s.reconnectHandler()

	return nil
}

// Exchange returns the exchange name
func (s *Stream) Exchange() string {
	return "bybit"
}

// Events returns the events channel
func (s *Stream) Events() <-chan price.Event {
	return s.events
}

// Stop closes the connection
func (s *Stream) Stop() error {
	s.logger.Info("stopping bybit stream")

	if s.cancel != nil {
		s.cancel()
	}

	s.connMu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.connMu.Unlock()

	s.wg.Wait()
	return nil
}

// IsConnected returns connection status
func (s *Stream) IsConnected() bool {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.conn != nil
}

func (s *Stream) connect() error {
	s.logger.Info("connecting to bybit websocket")

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(bybitWSURL, nil)
	if err != nil {
		return err
	}

	s.connMu.Lock()
	s.conn = conn
	s.connMu.Unlock()

	// Bybit handles ping/pong automatically
	// Just read messages without deadlines

	// Start goroutines for this session
	var sessCtx context.Context
	sessCtx, sessionCancel := context.WithCancel(s.ctx)

	s.wg.Add(2)
	go s.readMessages(sessCtx, conn, sessionCancel)
	go s.pingHandler(sessCtx, conn)

	s.logger.Info("connected to bybit websocket")

	// Resubscribe to all symbols
	s.subMu.Lock()
	for symbol := range s.subscriptions {
		if err := s.subscribe(symbol); err != nil {
			s.logger.Error("failed to resubscribe", "symbol", symbol, "error", err)
		}
	}
	s.subMu.Unlock()

	return nil
}

func (s *Stream) reconnectHandler() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.reconnectC:
			s.logger.Info("reconnecting in...", "delay", reconnectDelay)

			// Close old connection
			s.connMu.Lock()
			if s.conn != nil {
				s.conn.Close()
				s.conn = nil
			}
			s.connMu.Unlock()

			time.Sleep(reconnectDelay)

			// Reconnect
			if err := s.connect(); err != nil {
				s.logger.Error("reconnection failed", "error", err)
				// Trigger another reconnect
				select {
				case s.reconnectC <- struct{}{}:
				default:
				}
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
	}
}

func (s *Stream) readMessages(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc) {
	defer s.wg.Done()
	defer cancel()
	defer s.triggerReconnect()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, data, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() == nil {
					s.logger.Error("websocket read error (triggering reconnect)", "error", err)
				}
				return
			}

			if err := s.handleMessage(data); err != nil {
				s.logger.Error("message parsing error", "error", err)
			}
		}
	}
}

func (s *Stream) pingHandler(ctx context.Context, conn *websocket.Conn) {
	defer s.wg.Done()

	// Bybit requires ping every 20 seconds
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ping := pingMessage{Op: "ping"}

			s.connMu.RLock()
			if s.conn != nil {
				s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				err := s.conn.WriteJSON(ping)
				s.connMu.RUnlock()

				if err != nil {
					s.logger.Error("ping failed", "error", err)
					return
				}
			} else {
				s.connMu.RUnlock()
			}
		}
	}
}

func (s *Stream) Subscribe(symbol string) error {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	normalized := s.normalizer.Normalize(symbol)
	if s.subscriptions[normalized] {
		return nil
	}

	if err := s.subscribe(normalized); err != nil {
		return err
	}

	s.subscriptions[normalized] = true
	return nil
}

func (s *Stream) subscribe(normalized string) error {
	// Bybit format: BTCUSDT (same as our normalized format)
	bybitSymbol := normalized

	msg := subscribeMessage{
		Op:   "subscribe",
		Args: []string{fmt.Sprintf("tickers.%s", bybitSymbol)},
	}

	return s.sendMessage(msg)
}

func (s *Stream) Unsubscribe(symbol string) error {
	normalized := s.normalizer.Normalize(symbol)
	bybitSymbol := normalized

	s.subMu.Lock()
	defer s.subMu.Unlock()

	if !s.subscriptions[normalized] {
		return nil
	}

	msg := subscribeMessage{
		Op:   "unsubscribe",
		Args: []string{fmt.Sprintf("tickers.%s", bybitSymbol)},
	}

	if err := s.sendMessage(msg); err != nil {
		return err
	}

	delete(s.subscriptions, normalized)
	s.logger.Info("unsubscribed", "symbol", normalized)
	return nil
}

func (s *Stream) sendMessage(msg interface{}) error {
	s.connMu.RLock()
	conn := s.conn
	s.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection closed")
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := conn.WriteJSON(msg)

	if err != nil {
		s.logger.Warn("write failed, triggering reconnect", "error", err)
		s.triggerReconnect()
	}

	return err
}

func (s *Stream) handleMessage(data []byte) error {
	// Try to parse as ticker message
	var ticker tickerMessage
	if err := json.Unmarshal(data, &ticker); err == nil {
		if ticker.Topic != "" && ticker.Type == "snapshot" {
			return s.handleTickerUpdate(&ticker)
		}
	}

	// Try to parse as pong
	var pong pongMessage
	if err := json.Unmarshal(data, &pong); err == nil {
		if pong.Op == "pong" {
			return nil // Pong received
		}
	}

	// Ignore subscription confirmations and other messages
	return nil
}

func (s *Stream) handleTickerUpdate(msg *tickerMessage) error {
	markPrice := 0.0
	fmt.Sscanf(msg.Data.MarkPrice, "%f", &markPrice)

	if markPrice == 0 {
		return nil
	}

	event := price.Event{
		Exchange:  "bybit",
		Symbol:    s.normalizer.Normalize(msg.Data.Symbol),
		MarkPrice: markPrice,
		Timestamp: time.UnixMilli(msg.Ts),
	}

	select {
	case s.events <- event:
	default:
		s.logger.Warn("events channel full, dropping event")
	}

	return nil
}

// Ensure Stream implements exchange.Stream interface
var _ exchange.Stream = (*Stream)(nil)
