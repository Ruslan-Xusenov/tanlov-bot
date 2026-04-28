package handlers

import (
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/db"
	"tanlov-bot/keyboards"
)

// CheckUserSubscriptions checks if user is subscribed to all required channels.
// Checks are done concurrently for high speed.
func CheckUserSubscriptions(bot *tgbotapi.BotAPI, userID int64, forceCheck bool) (bool, []db.Channel, error) {

	channels, err := db.GetActiveChannels()
	if err != nil {
		return false, nil, err
	}
	if len(channels) == 0 {
		return true, nil, nil
	}

	var notSubscribed []db.Channel
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ch := range channels {
		wg.Add(1)
		go func(channel db.Channel) {
			defer wg.Done()
			member, err := bot.GetChatMember(tgbotapi.GetChatMemberConfig{
				ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
					SuperGroupUsername: channel.ChannelID,
					UserID:             userID,
				},
			})
			if err != nil {
				mu.Lock()
				notSubscribed = append(notSubscribed, channel)
				mu.Unlock()
				return
			}
			status := member.Status
			if status == "left" || status == "kicked" {
				mu.Lock()
				notSubscribed = append(notSubscribed, channel)
				mu.Unlock()
			}
		}(ch)
	}
	wg.Wait()

	return len(notSubscribed) == 0, notSubscribed, nil
}

// SendSubscriptionGate sends the mandatory subscription message with channel buttons
func SendSubscriptionGate(bot *tgbotapi.BotAPI, chatID int64, missing []db.Channel) {
	var sb strings.Builder
	sb.WriteString("⚠️ <b>Botdan foydalanish uchun quyidagi kanallarga obuna bo'ling:</b>\n\n")
	for i, ch := range missing {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, ch.ChannelName))
	}
	sb.WriteString("\n✅ Obuna bo'lgach, <b>Tekshirish</b> tugmasini bosing.")

	// Build inline buttons: one button per channel + check button
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, ch := range missing {
		url := ch.ChannelURL
		if url == "" {
			url = "https://t.me/" + strings.TrimPrefix(ch.ChannelID, "@")
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("👉 "+ch.ChannelName, url),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ Tekshirish", "check_sub"),
	))

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

// RequireSubscription is a middleware helper. Returns true if user passed the gate.
func RequireSubscription(bot *tgbotapi.BotAPI, chatID, userID int64, forceCheck bool) bool {
	ok, missing, err := CheckUserSubscriptions(bot, userID, forceCheck)
	if err != nil || !ok {
		// Proactively try to revoke in case they used to be subscribed and just left
		_ = db.RevokeReferral(userID)
		
		SendSubscriptionGate(bot, chatID, missing)
		return false
	}
	return true
}

// SendMainMenu sends the user main menu keyboard
func SendMainMenu(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboards.MainMenu()
	bot.Send(msg)
}

// SendAdminMenu sends the admin main menu keyboard
func SendAdminMenu(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboards.AdminMenu()
	bot.Send(msg)
}
