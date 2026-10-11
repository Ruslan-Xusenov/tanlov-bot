package handlers

import (
	"fmt"
	"log"
	"strconv"
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

			cfg := tgbotapi.GetChatMemberConfig{
				ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
					UserID: userID,
				},
			}
			rawID := strings.TrimSpace(channel.ChannelID)
			parts := strings.Fields(rawID)
			if len(parts) > 0 {
				rawID = parts[0]
			}
			if numID, err := strconv.ParseInt(rawID, 10, 64); err == nil {
				cfg.ChatID = numID
			} else {
				cfg.SuperGroupUsername = rawID
			}

			member, err := bot.GetChatMember(cfg)
			if err != nil {
				log.Printf("[sub] user %d is not member of %s (err: %v)", userID, rawID, err)
				mu.Lock()
				notSubscribed = append(notSubscribed, channel)
				mu.Unlock()
				return
			}
			status := member.Status
			if status != "creator" && status != "administrator" && status != "member" && status != "restricted" {
				log.Printf("[sub] user %d status is %s in %s", userID, status, rawID)
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
func SendSubscriptionGate(bot *tgbotapi.BotAPI, chatID int64, missing []db.Channel, customText ...string) {
	var sb strings.Builder
	
	if len(customText) > 0 && customText[0] != "" {
		sb.WriteString(customText[0])
	} else {
		sb.WriteString("⚠️ <b>Botdan foydalanish uchun quyidagi kanallarga obuna bo'ling:</b>\n\n")
		for i, ch := range missing {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, ch.ChannelName))
		}
		sb.WriteString("\n✅ Obuna bo'lgach, <b>Obuna bo'ldim ✅</b> tugmasini bosing.")
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	kb := BuildSubscriptionKeyboard(missing)
	msg.ReplyMarkup = kb
	bot.Send(msg)
}

// BuildSubscriptionKeyboard creates the inline keyboard for mandatory channels
func BuildSubscriptionKeyboard(missing []db.Channel) keyboards.ExtendedInlineKeyboardMarkup {
	var rows [][]keyboards.ExtendedInlineKeyboardButton
	for _, ch := range missing {
		url := ch.ChannelURL
		if url == "" {
			url = "https://t.me/" + strings.TrimPrefix(ch.ChannelID, "@")
		}
		rows = append(rows, keyboards.NewExtendedInlineKeyboardRow(
			keyboards.NewExtendedInlineKeyboardButtonURL(ch.ChannelName, url, ""), // Bu yerga kanal emoji ID sini qo'yish mumkin
		))
	}
	rows = append(rows, keyboards.NewExtendedInlineKeyboardRow(
		keyboards.NewExtendedInlineKeyboardButtonData("Obuna bo'ldim ✅", "check_sub", ""), // Bu yerga "tasdiqlash" jonli emoji ID sini qo'yasiz
	))
	return keyboards.NewExtendedInlineKeyboardMarkup(rows...)
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
