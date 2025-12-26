package tgbot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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

func (b *Bot) sendMessageWithMenu(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = b.getMainMenuKeyboard()
	if _, err := b.bot.Send(msg); err != nil {
		b.logger.Error("failed to send message", "error", err)
	}
}

func (b *Bot) sendMarkdownWithMenu(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = b.getMainMenuKeyboard()
	if _, err := b.bot.Send(msg); err != nil {
		b.logger.Error("failed to send message", "error", err)
	}
}
