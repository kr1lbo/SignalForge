package tgbot

import (
	"SignalForge/internal/domain/repository"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	// Get user
	user, err := b.userRepo.GetOrCreate(b.ctx, query.From.ID)
	if err != nil {
		b.logger.Error("failed to get user", "error", err)
		b.answerCallback(query.ID, "❌ Error occurred")
		return
	}

	data := query.Data

	// Handle different callback actions
	switch {
	case data == "toggle_telegram":
		b.handleToggleTelegram(query, user)
	case data == "toggle_pushover":
		b.handleTogglePushover(query, user)
	case data == "set_pushover_key":
		b.handleSetPushoverKey(query, user)
	case data == "view_pushover_key":
		b.handleViewPushoverKey(query, user)
	case strings.HasPrefix(data, "delete_"):
		b.handleDeleteCallback(query, user)
	default:
		b.answerCallback(query.ID, "Unknown action")
	}
}

func (b *Bot) handleToggleTelegram(query *tgbotapi.CallbackQuery, user *repository.User) {
	newStatus := !user.TelegramEnabled

	err := b.userRepo.UpdateSettings(b.ctx, user.ID, user.PushoverKey, user.PushoverEnabled, newStatus)
	if err != nil {
		b.logger.Error("failed to update telegram setting", "error", err)
		b.answerCallback(query.ID, "❌ Failed to update setting")
		return
	}

	// Update local user object
	user.TelegramEnabled = newStatus

	// Update the settings message
	text := "⚙️ *Notification Settings*\n\n" +
		"Configure how you receive alerts:"

	edit := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		text,
	)
	edit.ParseMode = "Markdown"
	edit.ReplyMarkup = b.getSettingsKeyboardPtr(user)

	if _, err := b.bot.Send(edit); err != nil {
		b.logger.Error("failed to update message", "error", err)
	}

	status := "disabled"
	if newStatus {
		status = "enabled"
	}
	b.answerCallback(query.ID, fmt.Sprintf("Telegram notifications %s", status))
}

func (b *Bot) handleTogglePushover(query *tgbotapi.CallbackQuery, user *repository.User) {
	newStatus := !user.PushoverEnabled

	// Check if trying to enable without key
	if newStatus && (user.PushoverKey == nil || *user.PushoverKey == "") {
		b.answerCallback(query.ID, "❌ Please set Pushover key first!")
		return
	}

	err := b.userRepo.UpdateSettings(b.ctx, user.ID, user.PushoverKey, newStatus, user.TelegramEnabled)
	if err != nil {
		b.logger.Error("failed to update pushover setting", "error", err)
		b.answerCallback(query.ID, "❌ Failed to update setting")
		return
	}

	// Update local user object
	user.PushoverEnabled = newStatus

	// Update the settings message
	text := "⚙️ *Notification Settings*\n\n" +
		"Configure how you receive alerts:"

	edit := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		text,
	)
	edit.ParseMode = "Markdown"
	edit.ReplyMarkup = b.getSettingsKeyboardPtr(user)

	if _, err := b.bot.Send(edit); err != nil {
		b.logger.Error("failed to update message", "error", err)
	}

	status := "disabled"
	if newStatus {
		status = "enabled"
	}
	b.answerCallback(query.ID, fmt.Sprintf("Pushover notifications %s", status))
}

func (b *Bot) handleSetPushoverKey(query *tgbotapi.CallbackQuery, user *repository.User) {
	// Set user state to await pushover key
	b.statesMu.Lock()
	b.userStates[query.Message.Chat.ID] = &UserState{
		State: "awaiting_pushover_key",
		Data:  make(map[string]interface{}),
	}
	b.statesMu.Unlock()

	b.answerCallback(query.ID, "")

	text := "🔑 *Set Pushover User Key*\n\n" +
		"Please send me your Pushover User Key.\n\n" +
		"You can find it at: https://pushover.net/\n" +
		"(Look for \"Your User Key\" on the dashboard)\n\n" +
		"Send /cancel to abort."

	b.sendMarkdown(query.Message.Chat.ID, text)
}

func (b *Bot) handleViewPushoverKey(query *tgbotapi.CallbackQuery, user *repository.User) {
	if user.PushoverKey == nil || *user.PushoverKey == "" {
		b.answerCallback(query.ID, "❌ No Pushover key set")
		return
	}

	// Show first and last 4 characters only for security
	key := *user.PushoverKey
	masked := key
	if len(key) > 8 {
		masked = key[:4] + "..." + key[len(key)-4:]
	}

	b.answerCallback(query.ID, "")

	text := fmt.Sprintf("🔑 *Your Pushover Key*\n\n`%s`\n\nFull key: `%s`", masked, key)
	b.sendMarkdown(query.Message.Chat.ID, text)
}

func (b *Bot) handleDeleteCallback(query *tgbotapi.CallbackQuery, user *repository.User) {
	// Extract alert ID from callback data (format: "delete_123")
	parts := strings.Split(query.Data, "_")
	if len(parts) != 2 {
		b.answerCallback(query.ID, "❌ Invalid delete command")
		return
	}

	alertID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		b.answerCallback(query.ID, "❌ Invalid alert ID")
		return
	}

	// Delete the alert
	b.deleteAlert(query.Message.Chat.ID, user, alertID)

	// Delete the message with the alert
	deleteConfig := tgbotapi.DeleteMessageConfig{
		ChatID:    query.Message.Chat.ID,
		MessageID: query.Message.MessageID,
	}

	if _, err := b.bot.Request(deleteConfig); err != nil {
		b.logger.Error("failed to delete message", "error", err)
	}

	b.answerCallback(query.ID, fmt.Sprintf("✅ Alert #%d deleted", alertID))
}

func (b *Bot) deleteAlert(chatID int64, user *repository.User, alertID int64) {
	// Get alert details before deleting
	alert, err := b.alertRepo.GetByID(b.ctx, alertID, user.ID)
	if err != nil {
		b.logger.Error("failed to get alert", "error", err)
		b.sendMessageWithMenu(chatID, "❌ Alert not found or already deleted")
		return
	}

	// Delete alert
	if err := b.alertRepo.Delete(b.ctx, alertID, user.ID); err != nil {
		b.logger.Error("failed to delete alert", "error", err)
		b.sendMessageWithMenu(chatID, "❌ Failed to delete alert")
		return
	}

	// Unsubscribe from watcher
	if err := b.watcher.Unsubscribe(alert.Exchange, alert.Symbol); err != nil {
		b.logger.Error("failed to unsubscribe from watcher",
			"exchange", alert.Exchange,
			"symbol", alert.Symbol,
			"error", err)
	}

	b.logger.Info("alert deleted", "alert_id", alertID, "user_id", user.ID)
}

func (b *Bot) answerCallback(callbackID string, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := b.bot.Request(callback); err != nil {
		b.logger.Error("failed to answer callback", "error", err)
	}
}
