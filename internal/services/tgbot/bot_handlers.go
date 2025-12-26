package tgbot

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"SignalForge/internal/domain/repository"
)

func (b *Bot) handleStart(message *tgbotapi.Message, user *repository.User) {
	text := "👋 Welcome to *SignalForge*!\n\n" +
		"I'll notify you when cryptocurrency prices hit your targets.\n\n" +
		"Use the menu below to get started! 👇"

	b.sendMarkdownWithMenu(message.Chat.ID, text)
}

func (b *Bot) handleHelp(message *tgbotapi.Message) {
	text := "*SignalForge Help*\n\n" +
		"*Creating alerts:*\n" +
		"`/new <exchange> <symbol> <price> <direction> [notes]`\n\n" +
		"*Parameters:*\n" +
		"• exchange: `gate` (Gate.io) - more coming soon!\n" +
		"• symbol: Trading pair (e.g., `BTCUSDT`, `ETHUSDT`)\n" +
		"• price: Target price\n" +
		"• direction: `above` or `below`\n" +
		"• notes: Optional notes\n\n" +
		"*Examples:*\n" +
		"`/new gate BTCUSDT 100000 above`\n" +
		"`/new gate ETHUSDT 3000 below My ETH buy target`\n\n" +
		"*Or use the menu:*\n" +
		"📋 My Alerts - View all active alerts\n" +
		"➕ New Alert - Quick guide to create\n" +
		"⚙️ Settings - Manage notifications"

	b.sendMarkdownWithMenu(message.Chat.ID, text)
}

func (b *Bot) handleNewAlertButton(message *tgbotapi.Message) {
	text := "➕ *Create New Alert*\n\n" +
		"Use this command format:\n\n" +
		"`/new <exchange> <symbol> <price> <direction>`\n\n" +
		"*Example:*\n" +
		"`/new gate BTCUSDT 100000 above`\n\n" +
		"*Available exchanges:*\n" +
		"• gate - Gate.io ✅\n\n" +
		"*Coming soon:*\n" +
		"• bybit - Bybit 🔜\n" +
		"• binance - Binance 🔜\n\n" +
		"*Direction:*\n" +
		"• above - Alert when price goes above target\n" +
		"• below - Alert when price goes below target"

	b.sendMarkdownWithMenu(message.Chat.ID, text)
}

func (b *Bot) handleNew(message *tgbotapi.Message, user *repository.User) {
	args := strings.Fields(message.CommandArguments())

	if len(args) < 4 {
		b.sendMessageWithMenu(message.Chat.ID,
			"❌ Invalid format.\n\n"+
				"Usage: `/new <exchange> <symbol> <price> <direction> [notes]`\n"+
				"Example: `/new gate BTCUSDT 100000 above`\n\n"+
				"Or tap ➕ New Alert button for help")
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

	// Validate exchange - only implemented ones
	if exchange != "gate" {
		if exchange == "bybit" || exchange == "binance" {
			b.sendMessageWithMenu(message.Chat.ID,
				"❌ "+strings.Title(exchange)+" is not yet supported.\n\n"+
					"Currently available:\n• gate - Gate.io ✅\n\n"+
					"Coming soon:\n• bybit - Bybit 🔜\n• binance - Binance 🔜")
		} else {
			b.sendMessageWithMenu(message.Chat.ID,
				"❌ Unknown exchange.\n\n"+
					"Currently available:\n• gate - Gate.io ✅")
		}
		return
	}

	// Validate direction
	if direction != "above" && direction != "below" {
		b.sendMessageWithMenu(message.Chat.ID, "❌ Invalid direction. Use: above or below")
		return
	}

	// Parse price
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price <= 0 {
		b.sendMessageWithMenu(message.Chat.ID, "❌ Invalid price. Must be a positive number.")
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
		b.sendMessageWithMenu(message.Chat.ID, "❌ Failed to create alert. Please try again.")
		return
	}

	// Subscribe to price updates
	if err := b.watcher.Subscribe(exchange, symbol); err != nil {
		b.logger.Error("failed to subscribe to watcher",
			"exchange", exchange,
			"symbol", symbol,
			"error", err)
	}

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

	b.sendMarkdownWithMenu(message.Chat.ID, text)
}

func (b *Bot) handleListButton(message *tgbotapi.Message, user *repository.User) {
	// Fetch only ACTIVE alerts (not fired)
	alerts, err := b.alertRepo.List(b.ctx, user.ID, repository.AlertFilter{})
	if err != nil {
		b.logger.Error("failed to list alerts", "error", err)
		b.sendMessageWithMenu(message.Chat.ID, "❌ Failed to fetch alerts.")
		return
	}

	// Filter only active (not fired)
	activeAlerts := make([]*repository.Alert, 0)
	for _, alert := range alerts {
		if alert.FiredAt == nil && alert.IsActive {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	if len(activeAlerts) == 0 {
		b.sendMessageWithMenu(message.Chat.ID, "You have no active alerts.\n\nTap ➕ New Alert to create one!")
		return
	}

	// Send each alert as separate message with delete button
	b.sendMessageWithMenu(message.Chat.ID, fmt.Sprintf("📋 *Your Active Alerts* (%d)\n", len(activeAlerts)))

	for _, alert := range activeAlerts {
		text := fmt.Sprintf(
			"*#%d* - %s\n"+
				"Exchange: %s\n"+
				"Price: %s %s",
			alert.ID,
			alert.Symbol,
			alert.Exchange,
			formatPrice(alert.Price),
			alert.Direction,
		)

		if alert.Notes != nil && *alert.Notes != "" {
			text += fmt.Sprintf("\nNotes: %s", *alert.Notes)
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = b.getAlertDeleteButton(alert.ID)

		if _, err := b.bot.Send(msg); err != nil {
			b.logger.Error("failed to send alert", "error", err)
		}
	}
}

func (b *Bot) handleDeleteCommand(message *tgbotapi.Message, user *repository.User) {
	args := strings.Fields(message.CommandArguments())

	if len(args) == 0 {
		b.sendMessageWithMenu(message.Chat.ID, "❌ Usage: `/delete <alert_id>`\n\nUse 📋 My Alerts to see your alerts")
		return
	}

	alertID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		b.sendMessageWithMenu(message.Chat.ID, "❌ Invalid alert ID")
		return
	}

	b.deleteAlert(message.Chat.ID, user, alertID)
}

func (b *Bot) handleSettingsButton(message *tgbotapi.Message, user *repository.User) {
	text := "⚙️ *Notification Settings*\n\n" +
		"Configure how you receive alerts:"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = b.getSettingsKeyboard(user)

	if _, err := b.bot.Send(msg); err != nil {
		b.logger.Error("failed to send settings", "error", err)
	}
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
