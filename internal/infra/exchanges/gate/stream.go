package gate

import (
	"SignalForge/internal/domain/price"
	"SignalForge/internal/infra/symbol"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	gateWSURL = "wss://fx-ws.gateio.ws/v4/ws/usdt"

	maxSubscriptionsPerConn = 100
	reconnectDelay          = 5 * time.Second
)

type Stream struct {
	logger     *slog.Logger
	normalizer *symbol.Normalizer

	conn   *websocket.Conn
	connMu sync.RWMutex

	subscriptions map[string]bool
	subMu         sync.RWMutex

	events chan price.Event

	ctx    context.Context
	cancel context.CancelFunc

	// sessionCancel отменяет горутины конкретного подключения
	sessionCancel context.CancelFunc
	wg            sync.WaitGroup
	reconnectC    chan struct{}
}

func New(logger *slog.Logger, normalizer *symbol.Normalizer) *Stream {
	return &Stream{
		logger:        logger.With("exchange", "gate"),
		normalizer:    normalizer,
		subscriptions: make(map[string]bool),
		events:        make(chan price.Event, 100),
		reconnectC:    make(chan struct{}, 1),
	}
}

func (s *Stream) Exchange() string {
	return "gate"
}

func (s *Stream) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.logger.Info("starting gate.io stream")

	// Первый запуск
	if err := s.connect(); err != nil {
		s.logger.Error("initial connection failed, starting retry loop", "error", err)
		s.triggerReconnect()
	}

	// Обработчик реконнекта живет весь срок службы Stream
	s.wg.Add(1)
	go s.reconnectHandler()

	<-s.ctx.Done()
	s.cleanup()
	s.wg.Wait()
	return nil
}

func (s *Stream) connect() error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.sessionCancel != nil {
		s.sessionCancel()
	}
	if s.conn != nil {
		s.conn.Close()
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	// Gate.io requires this header for decimal size support
	headers := map[string][]string{
		"X-Gate-Size-Decimal": {"1"},
	}

	conn, _, err := dialer.Dial(gateWSURL, headers)
	if err != nil {
		return err
	}

	// ВАЖНО: Обработка понгов от биржи
	conn.SetPongHandler(func(string) error {
		// Продлеваем жизнь соединению при получении ответа
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// ВАЖНО: Автоматический ответ на ping от сервера
	conn.SetPingHandler(func(appData string) error {
		// Gate.io отправляет WebSocket Ping, мы должны ответить Pong
		err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
		if err != nil {
			s.logger.Error("failed to send pong", "error", err)
		}
		// Продлеваем read deadline
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	s.conn = conn
	var sessCtx context.Context
	sessCtx, s.sessionCancel = context.WithCancel(s.ctx)

	s.wg.Add(2)
	go s.readMessages(sessCtx, conn)
	go s.pingHandler(sessCtx, conn)

	// Автоматическое восстановление подписок
	go s.resubscribeAll()

	return nil
}

func (s *Stream) IsConnected() bool {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.conn != nil
}

func (s *Stream) resubscribeAll() {
	s.subMu.RLock()
	symbols := make([]string, 0, len(s.subscriptions))
	for sym := range s.subscriptions {
		symbols = append(symbols, sym)
	}
	s.subMu.RUnlock()

	for _, sym := range symbols {
		if err := s.subscribe(sym); err != nil {
			s.logger.Error("resubscribe failed", "symbol", sym, "error", err)
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
	gateSymbol := s.normalizer.ToExchangeFormat("gate", normalized)
	msg := subscribeMessage{
		Time:    time.Now().Unix(),
		Channel: "futures.tickers",
		Event:   "subscribe",
		Payload: []string{gateSymbol},
	}
	return s.sendMessage(msg)
}

// Unsubscribe удаляет символ из списка подписок
func (s *Stream) Unsubscribe(symbol string) error {
	normalized := s.normalizer.Normalize(symbol)
	gateSymbol := s.normalizer.ToExchangeFormat("gate", normalized)

	s.subMu.Lock()
	defer s.subMu.Unlock()

	if !s.subscriptions[normalized] {
		return nil // Не был подписан
	}

	// Отправляем сообщение об отписке
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

func (s *Stream) readMessages(ctx context.Context, conn *websocket.Conn) {
	defer s.wg.Done()
	defer s.triggerReconnect()

	// Если за 60 секунд не пришло ни одного сообщения - соединение мертво
	readWait := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(readWait))
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

	// Gate.io сам отправляет WebSocket Ping каждые ~20 секунд
	// Мы просто отвечаем через SetPingHandler
	// Этот handler нужен только для application-level ping (опционально)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Опциональный application-level ping для проверки соединения
			pingMsg := pingMessage{
				Time:    time.Now().Unix(),
				Channel: "futures.ping",
			}

			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(pingMsg); err != nil {
				s.logger.Error("application ping failed", "error", err)
				return
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
			s.logger.Info("reconnecting in...", "delay", reconnectDelay)
			time.Sleep(reconnectDelay)

			if err := s.connect(); err != nil {
				s.logger.Error("reconnect failed", "error", err)
				s.triggerReconnect()
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
		// Уже запланировано
	}
}

func (s *Stream) sendMessage(msg interface{}) error {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	if s.conn == nil {
		return fmt.Errorf("connection closed")
	}
	return s.conn.WriteJSON(msg)
}

func (s *Stream) handleMessage(data []byte) error {
	var base baseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}

	switch base.Event {
	case "update":
		if base.Channel == "futures.tickers" {
			return s.handleTickerUpdate(data)
		}
	case "subscribe", "unsubscribe":
		// Subscription confirmations
		return nil
	}

	// Handle pong response from application-level ping
	if base.Channel == "futures.pong" {
		return nil
	}

	return nil
}

func (s *Stream) handleTickerUpdate(data []byte) error {
	var msg tickerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	if len(msg.Result) == 0 {
		return nil
	}

	ticker := msg.Result[0]
	markPrice := 0.0
	fmt.Sscanf(ticker.MarkPrice, "%f", &markPrice)

	event := price.Event{
		Exchange:  "gate",
		Symbol:    s.normalizer.FromExchangeFormat("gate", ticker.Contract),
		MarkPrice: markPrice,
		Timestamp: time.Now(),
	}

	select {
	case s.events <- event:
	default:
		s.logger.Warn("buffer full, drop event", "symbol", event.Symbol)
	}
	return nil
}

func (s *Stream) cleanup() {
	s.connMu.Lock()
	if s.sessionCancel != nil {
		s.sessionCancel()
	}
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.connMu.Unlock()
}

func (s *Stream) Events() <-chan price.Event { return s.events }

// Типы сообщений (оставлены без изменений)
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
	Result []tickerData `json:"result"`
}
type tickerData struct {
	Contract  string `json:"contract"`
	MarkPrice string `json:"mark_price"`
}
