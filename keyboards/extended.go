package keyboards

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// ExtendedInlineKeyboardMarkup adds support for Custom Emojis in Telegram Bot API 9.4+
type ExtendedInlineKeyboardMarkup struct {
	InlineKeyboard [][]ExtendedInlineKeyboardButton `json:"inline_keyboard"`
}

type ExtendedInlineKeyboardButton struct {
	tgbotapi.InlineKeyboardButton
	IconCustomEmojiId string `json:"icon_custom_emoji_id,omitempty"`
}

// Helper to create an extended row
func NewExtendedInlineKeyboardRow(buttons ...ExtendedInlineKeyboardButton) []ExtendedInlineKeyboardButton {
	return buttons
}

// Helper to create the extended markup
func NewExtendedInlineKeyboardMarkup(rows ...[]ExtendedInlineKeyboardButton) ExtendedInlineKeyboardMarkup {
	return ExtendedInlineKeyboardMarkup{InlineKeyboard: rows}
}

// Helper to create an extended button with data and a custom emoji ID
func NewExtendedInlineKeyboardButtonData(text, data, emojiID string) ExtendedInlineKeyboardButton {
	cb := data
	return ExtendedInlineKeyboardButton{
		InlineKeyboardButton: tgbotapi.InlineKeyboardButton{
			Text:         text,
			CallbackData: &cb,
		},
		IconCustomEmojiId: emojiID,
	}
}

// Helper to create an extended button with URL and a custom emoji ID
func NewExtendedInlineKeyboardButtonURL(text, url, emojiID string) ExtendedInlineKeyboardButton {
	return ExtendedInlineKeyboardButton{
		InlineKeyboardButton: tgbotapi.InlineKeyboardButton{
			Text: text,
			URL:  &url,
		},
		IconCustomEmojiId: emojiID,
	}
}
