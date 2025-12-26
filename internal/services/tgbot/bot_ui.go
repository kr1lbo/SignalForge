package tgbot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"SignalForge/internal/domain/repository"
)

// Main menu keyboard (always shown at bottom)
func (b *Bot) getMainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📋 My Alerts"),
			tgbotapi.NewKeyboardButton("➕ New Alert"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("⚙️ Settings"),
		),
	)
}

// Settings inline keyboard
func (b *Bot) getSettingsKeyboard(user *repository.User) tgbotapi.InlineKeyboardMarkup {
	telegramStatus := "❌"
	if user.TelegramEnabled {
		telegramStatus = "✅"
	}

	pushoverStatus := "❌"
	if user.PushoverEnabled {
		pushoverStatus = "✅"
	}

	var pushoverKeyText string
	if user.PushoverKey == nil || *user.PushoverKey == "" {
		pushoverKeyText = "🔑 Set Pushover Key"
	} else {
		pushoverKeyText = "🔑 Change Pushover Key"
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				telegramStatus+" Telegram",
				"toggle_telegram",
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				pushoverStatus+" Pushover",
				"toggle_pushover",
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				pushoverKeyText,
				"set_pushover_key",
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"🔍 View Pushover Key",
				"view_pushover_key",
			),
		),
	)
}

// getSettingsKeyboardPtr returns pointer for editing messages
func (b *Bot) getSettingsKeyboardPtr(user *repository.User) *tgbotapi.InlineKeyboardMarkup {
	keyboard := b.getSettingsKeyboard(user)
	return &keyboard
}

// Alert list inline keyboard (delete button for each alert)
func (b *Bot) getAlertDeleteButton(alertID int64) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"🗑 Delete",
				fmt.Sprintf("delete_%d", alertID),
			),
		),
	)
}
