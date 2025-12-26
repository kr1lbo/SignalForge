package tgbot

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"SignalForge/internal/domain/repository"
)

func (b *Bot) handleDialogState(message *tgbotapi.Message, user *repository.User, state *UserState) {
	// Handle /cancel command
	if message.IsCommand() && message.Command() == "cancel" {
		b.statesMu.Lock()
		delete(b.userStates, message.Chat.ID)
		b.statesMu.Unlock()

		b.sendMessageWithMenu(message.Chat.ID, "❌ Cancelled.")
		return
	}

	switch state.State {
	case "awaiting_pushover_key":
		b.handlePushoverKeyInput(message, user, state)
	default:
		// Unknown state, clear it
		b.statesMu.Lock()
		delete(b.userStates, message.Chat.ID)
		b.statesMu.Unlock()
	}
}

func (b *Bot) handlePushoverKeyInput(message *tgbotapi.Message, user *repository.User, state *UserState) {
	key := strings.TrimSpace(message.Text)

	// Validate key format (30 characters, alphanumeric)
	if len(key) != 30 {
		b.sendMessage(message.Chat.ID, "❌ Invalid Pushover key. It should be 30 characters long.\n\nTry again or /cancel")
		return
	}

	// Update user settings with new key
	err := b.userRepo.UpdateSettings(b.ctx, user.ID, &key, user.PushoverEnabled, user.TelegramEnabled)
	if err != nil {
		b.logger.Error("failed to update pushover key", "error", err)
		b.sendMessageWithMenu(message.Chat.ID, "❌ Failed to save key. Please try again.")

		// Clear state
		b.statesMu.Lock()
		delete(b.userStates, message.Chat.ID)
		b.statesMu.Unlock()
		return
	}

	// Clear state
	b.statesMu.Lock()
	delete(b.userStates, message.Chat.ID)
	b.statesMu.Unlock()

	b.sendMarkdownWithMenu(message.Chat.ID,
		"✅ *Pushover key saved!*\n\n"+
			"You can now enable Pushover notifications in ⚙️ Settings")
}
