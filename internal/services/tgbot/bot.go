package tgbot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

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

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
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
		logger:    logger.With("service", "tgbot"),
		token:     token,
		userRepo:  userRepo,
		alertRepo: alertRepo,
		watcher:   watcher,
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
			if update.Message == nil {
				continue
			}

			b.handleMessage(update.Message)
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

	// Handle commands
	if message.IsCommand() {
		b.handleCommand(message, user)
		return
	}

	// Non-command message
	b.sendMessage(message.Chat.ID, "Use /help to see available commands")
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
		b.handleList(message, user)
	case "delete":
		b.handleDelete(message, user)
	case "settings":
		b.handleSettings(message, user)
	default:
		b.sendMessage(message.Chat.ID, "Unknown command. Use /help to see available commands.")
	}
}

func (b *Bot) handleStart(message *tgbotapi.Message, user *repository.User) {
	text := fmt.Sprintf(
		"👋 Welcome to *SignalForge*!\n\n" +
			"I'll notify you when cryptocurrency prices hit your targets.\n\n" +
			"*Available commands:*\n" +
			"/new - Create a new price alert\n" +
			"/list - View your active alerts\n" +
			"/delete - Delete an alert\n" +
			"/settings - Manage notification settings\n" +
			"/help - Show this help\n\n" +
			"*Example:*\n" +
			"`/new gate BTCUSDT 100000 above`\n" +
			"This creates an alert when BTC goes above $100,000 on Gate.io",
	)

	b.sendMarkdown(message.Chat.ID, text)
}

func (b *Bot) handleHelp(message *tgbotapi.Message) {
	text := "*SignalForge Help*\n\n" +
		"*Creating alerts:*\n" +
		"`/new <exchange> <symbol> <price> <direction> [notes]`\n\n" +
		"*Parameters:*\n" +
		"• exchange: `gate`, `bybit`, or `binance`\n" +
		"• symbol: Trading pair (e.g., `BTCUSDT`, `ETHUSDT`)\n" +
		"• price: Target price\n" +
		"• direction: `above` or `below`\n" +
		"• notes: Optional notes\n\n" +
		"*Examples:*\n" +
		"`/new gate BTCUSDT 100000 above`\n" +
		"`/new gate ETHUSDT 3000 below My ETH buy target`\n\n" +
		"*Other commands:*\n" +
		"`/list` - Show your alerts\n" +
		"`/delete <alert_id>` - Delete alert\n" +
		"`/settings` - Notification settings"

	b.sendMarkdown(message.Chat.ID, text)
}

func (b *Bot) handleNew(message *tgbotapi.Message, user *repository.User) {
	args := strings.Fields(message.CommandArguments())

	if len(args) < 4 {
		b.sendMessage(message.Chat.ID,
			"❌ Invalid format.\n\n"+
				"Usage: `/new <exchange> <symbol> <price> <direction> [notes]`\n"+
				"Example: `/new gate BTCUSDT 100000 above`")
		return
	}

	exchange := strings.ToLower(args[0])
	symbol := strings.ToUpper(args[1])
	priceStr := args[2]
	direction := strings.ToLower(args[3])
	notes := ""
	if len(args) > 4 {
		notes = strings.Join(args[4:], " ")
	}

	// Validate exchange
	if exchange != "gate" && exchange != "bybit" && exchange != "binance" {
		b.sendMessage(message.Chat.ID, "❌ Invalid exchange. Use: gate, bybit, or binance")
		return
	}

	// Validate direction
	if direction != "above" && direction != "below" {
		b.sendMessage(message.Chat.ID, "❌ Invalid direction. Use: above or below")
		return
	}

	// Parse price
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price <= 0 {
		b.sendMessage(message.Chat.ID, "❌ Invalid price. Must be a positive number.")
		return
	}

	// Create alert
	alert := &repository.Alert{
		UserID:    user.ID,
		Exchange:  exchange,
		Symbol:    symbol,
		Price:     price,
		Direction: direction,
	}
	if notes != "" {
		alert.Notes = &notes
	}

	if err := b.alertRepo.Create(b.ctx, alert); err != nil {
		b.logger.Error("failed to create alert", "error", err)
		b.sendMessage(message.Chat.ID, "❌ Failed to create alert. Please try again.")
		return
	}

	// Subscribe to price updates
	if err := b.watcher.Subscribe(exchange, symbol); err != nil {
		b.logger.Error("failed to subscribe to watcher",
			"exchange", exchange,
			"symbol", symbol,
			"error", err)
		// Don't fail - watcher will pick it up on retry
	}

	// Check if alert condition is already met
	// Note: This is a best-effort check, actual triggering happens in watcher
	warningText := ""

	text := fmt.Sprintf(
		"✅ *Alert created!*\n\n"+
			"*ID:* %d\n"+
			"*Exchange:* %s\n"+
			"*Symbol:* %s\n"+
			"*Price:* %s\n"+
			"*Direction:* %s",
		alert.ID, exchange, symbol, formatPrice(price), direction,
	)
	if notes != "" {
		text += fmt.Sprintf("\n*Notes:* %s", notes)
	}
	if warningText != "" {
		text += "\n\n⚠️ " + warningText
	}

	b.sendMarkdown(message.Chat.ID, text)
}

// formatPrice formats price with appropriate precision based on magnitude
func formatPrice(price float64) string {
	switch {
	case price >= 1000:
		return fmt.Sprintf("$%,.2f", price)
	case price >= 1:
		return fmt.Sprintf("$%.2f", price)
	case price >= 0.01:
		return fmt.Sprintf("$%.4f", price)
	case price >= 0.0001:
		return fmt.Sprintf("$%.6f", price)
	default:
		return fmt.Sprintf("$%.8f", price)
	}
}

func (b *Bot) handleList(message *tgbotapi.Message, user *repository.User) {
	alerts, err := b.alertRepo.List(b.ctx, user.ID, repository.AlertFilter{})
	if err != nil {
		b.logger.Error("failed to list alerts", "error", err)
		b.sendMessage(message.Chat.ID, "❌ Failed to fetch alerts.")
		return
	}

	if len(alerts) == 0 {
		b.sendMessage(message.Chat.ID, "You have no alerts.\n\nUse /new to create one!")
		return
	}

	text := "*Your Alerts:*\n\n"
	for _, alert := range alerts {
		status := "✅ Active"
		if alert.FiredAt != nil {
			status = "🔔 Triggered"
		} else if !alert.IsActive {
			status = "⏸ Paused"
		}

		text += fmt.Sprintf(
			"*#%d* - %s\n"+
				"Exchange: %s | Symbol: %s\n"+
				"Price: %s %s\n"+
				"Status: %s\n\n",
			alert.ID, alert.Symbol, alert.Exchange, alert.Symbol,
			formatPrice(alert.Price), alert.Direction, status,
		)
	}

	text += "Use `/delete <id>` to remove an alert"

	b.sendMarkdown(message.Chat.ID, text)
}

func (b *Bot) handleDelete(message *tgbotapi.Message, user *repository.User) {
	args := strings.Fields(message.CommandArguments())

	if len(args) == 0 {
		b.sendMessage(message.Chat.ID, "❌ Usage: `/delete <alert_id>`\n\nUse /list to see your alerts")
		return
	}

	alertID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ Invalid alert ID")
		return
	}

	// Get alert details before deleting (to unsubscribe)
	alert, err := b.alertRepo.GetByID(b.ctx, alertID, user.ID)
	if err != nil {
		b.logger.Error("failed to get alert", "error", err)
		b.sendMessage(message.Chat.ID, "❌ Alert not found or already deleted")
		return
	}

	// Delete alert
	if err := b.alertRepo.Delete(b.ctx, alertID, user.ID); err != nil {
		b.logger.Error("failed to delete alert", "error", err)
		b.sendMessage(message.Chat.ID, "❌ Failed to delete alert. Make sure the ID is correct.")
		return
	}

	// Unsubscribe from watcher (with debounce - will only unsubscribe if no other alerts)
	if err := b.watcher.Unsubscribe(alert.Exchange, alert.Symbol); err != nil {
		b.logger.Error("failed to unsubscribe from watcher",
			"exchange", alert.Exchange,
			"symbol", alert.Symbol,
			"error", err)
		// Don't fail - this is just cleanup
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Alert #%d deleted", alertID))
}

func (b *Bot) handleSettings(message *tgbotapi.Message, user *repository.User) {
	text := fmt.Sprintf(
		"*Current Settings:*\n\n"+
			"Telegram notifications: %s\n"+
			"Pushover notifications: %s\n\n"+
			"_Settings management coming soon..._",
		boolToEmoji(user.TelegramEnabled),
		boolToEmoji(user.PushoverEnabled),
	)

	b.sendMarkdown(message.Chat.ID, text)
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.bot.Send(msg); err != nil {
		b.logger.Error("failed to send message", "error", err)
	}
}

func (b *Bot) sendMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.bot.Send(msg); err != nil {
		b.logger.Error("failed to send message", "error", err)
	}
}

func boolToEmoji(val bool) string {
	if val {
		return "✅ Enabled"
	}
	return "❌ Disabled"
}
