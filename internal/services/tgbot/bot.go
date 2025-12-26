package tgbot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"SignalForge/internal/domain/repository"
)

// Bot represents the Telegram bot service
type Bot struct {
	logger *slog.Logger
	token  string
	bot    *tgbotapi.BotAPI

	// Repositories
	userRepo  repository.UserRepository
	alertRepo repository.AlertRepository

	// Services
	watcher Watcher

	// User states for dialogs (chat_id -> state)
	userStates map[int64]*UserState
	statesMu   sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// UserState tracks user's current dialog state
type UserState struct {
	State string // "awaiting_pushover_key"
	Data  map[string]interface{}
}

// Watcher interface for subscribing to symbols
type Watcher interface {
	Subscribe(exchange, symbol string) error
	Unsubscribe(exchange, symbol string) error
}

// New creates a new Telegram bot
func New(
	logger *slog.Logger,
	token string,
	userRepo repository.UserRepository,
	alertRepo repository.AlertRepository,
	watcher Watcher,
) *Bot {
	return &Bot{
		logger:     logger.With("service", "tgbot"),
		token:      token,
		userRepo:   userRepo,
		alertRepo:  alertRepo,
		watcher:    watcher,
		userStates: make(map[int64]*UserState),
	}
}

// Start begins processing Telegram updates
func (b *Bot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.logger.Info("telegram bot starting")

	// Initialize bot
	bot, err := tgbotapi.NewBotAPI(b.token)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}
	b.bot = bot

	b.logger.Info("authorized on account", "username", bot.Self.UserName)

	// Setup updates
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Process updates
	for {
		select {
		case <-b.ctx.Done():
			return nil
		case update := <-updates:
			if update.Message != nil {
				b.handleMessage(update.Message)
			} else if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
			}
		}
	}
}

// Stop gracefully stops the bot
func (b *Bot) Stop() error {
	b.logger.Info("telegram bot stopping")

	if b.cancel != nil {
		b.cancel()
	}

	if b.bot != nil {
		b.bot.StopReceivingUpdates()
	}

	return nil
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	b.logger.Debug("received message",
		"user_id", message.From.ID,
		"username", message.From.UserName,
		"text", message.Text)

	// Get or create user
	user, err := b.userRepo.GetOrCreate(b.ctx, message.From.ID)
	if err != nil {
		b.logger.Error("failed to get/create user", "error", err)
		b.sendMessage(message.Chat.ID, "❌ Internal error. Please try again.")
		return
	}

	// Check if user is in a dialog state
	b.statesMu.RLock()
	state, hasState := b.userStates[message.Chat.ID]
	b.statesMu.RUnlock()

	if hasState {
		b.handleDialogState(message, user, state)
		return
	}

	// Handle commands
	if message.IsCommand() {
		b.handleCommand(message, user)
		return
	}

	// Handle button presses (Reply keyboard)
	switch message.Text {
	case "📋 My Alerts":
		b.handleListButton(message, user)
	case "➕ New Alert":
		b.handleNewAlertButton(message)
	case "⚙️ Settings":
		b.handleSettingsButton(message, user)
	default:
		b.sendMessageWithMenu(message.Chat.ID, "Use the menu below or /help for commands")
	}
}

func (b *Bot) handleCommand(message *tgbotapi.Message, user *repository.User) {
	switch message.Command() {
	case "start":
		b.handleStart(message, user)
	case "help":
		b.handleHelp(message)
	case "new":
		b.handleNew(message, user)
	case "list":
		b.handleListButton(message, user)
	case "delete":
		b.handleDeleteCommand(message, user)
	case "settings":
		b.handleSettingsButton(message, user)
	default:
		b.sendMessage(message.Chat.ID, "Unknown command. Use /help to see available commands.")
	}
}
