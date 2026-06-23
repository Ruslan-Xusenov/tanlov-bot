package handlers

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/db"
	"tanlov-bot/utils"
)

// HandleUserMessage handles regular (non-admin) user button presses
func HandleUserMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, botUsername string) {
	chatID := msg.Chat.ID
	userID := msg.From.ID

	// Touch last active
	db.TouchUserActivity(userID)

	// Subscription gate for all user actions
	if !RequireSubscription(bot, chatID, userID, false) {
		return
	}

	switch msg.Text {
	case "Top taklif qilganlar":
		handleRating(bot, chatID)
	case "Taklif havolam":
		handleReferral(bot, chatID, userID, botUsername)
	case "Qo`llanma":
		handleQullanma(bot, chatID, nil)
	case "Ballarim":
		handleBallarim(bot, chatID, userID)
	}
}

func handleQullanma(bot *tgbotapi.BotAPI, chatID int64, markup interface{}) {
	qullanmaText, _ := db.GetSetting("qullanma_text")
	if qullanmaText == "" {
		qullanmaText = "📄 <b>Qo'llanma</b>\nSizga berilgan referal havoladan nusxa oling va do'stlaringizga yuboring."
	}
	
	msg := tgbotapi.NewMessage(chatID, qullanmaText)
	msg.ParseMode = "HTML"
	if markup != nil {
		msg.ReplyMarkup = markup
	}
	bot.Send(msg)
}

func handleBallarim(bot *tgbotapi.BotAPI, chatID, userID int64) {
	user, err := db.GetUser(userID)
	count := 0
	if err == nil && user != nil {
		count = user.ReferralCount
	}
	send(bot, chatID, fmt.Sprintf("👥 Siz chaqirgan foydalanuvchilar: <b>%d ta</b>", count))
}

// handleRating shows the top 10 referrers leaderboard
func handleRating(bot *tgbotapi.BotAPI, chatID int64) {
	users, err := db.GetTopReferrers(10)
	if err != nil {
		log.Printf("[user] failed to get top referrers: %v", err)
		send(bot, chatID, "❌ Xatolik yuz berdi. Iltimos, keyinroq urinib ko'ring.")
		return
	}

	if len(users) == 0 {
		send(bot, chatID, "📊 <b>Reyting</b>\n\nHali hech kim ro'yxatdan o'tmagan.")
		return
	}

	medals := []string{"🥇", "🥈", "🥉"}
	var sb strings.Builder
	sb.WriteString("🏆 <b>Eng ko'p referal chaqirganlar:</b>\n\n")

	for i, u := range users {
		medal := fmt.Sprintf("%d.", i+1)
		if i < len(medals) {
			medal = medals[i]
		}

		name := u.FullName
		if u.Username != "" {
			name = "@" + u.Username
		}
		if name == "" {
			name = fmt.Sprintf("User#%d", u.ID)
		}

		sb.WriteString(fmt.Sprintf("%s <b>%s</b> — %d ta referal\n", medal, name, u.ReferralCount))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	bot.Send(msg)
}

// handleReferral shows ad text + inline button with deep-link to the bot
func handleReferral(bot *tgbotapi.BotAPI, chatID, userID int64, botUsername string) {
	user, err := db.GetUser(userID)
	if err != nil {
		send(bot, chatID, "❌ Xatolik yuz berdi.")
		return
	}

	link := utils.BuildReferralLink(botUsername, userID)
	count := 0
	if user != nil {
		count = user.ReferralCount
	}

	// Get admin-configured ad text
	adText, _ := db.GetSetting("referral_ad_text")
	if adText == "" {
		adText = "🚀 Do'stingizni taklif qiling!\n\n👇 Pastdagi tugmani bosing:\n\n🔗 <b>Sizning referal havolangiz:</b>\n{link}\n\n👥 Siz chaqirgan foydalanuvchilar: <b>{count} ta</b>"
	}

	var finalMessage string
	if strings.Contains(adText, "{link}") || strings.Contains(adText, "{count}") {
		finalMessage = strings.ReplaceAll(adText, "{link}", link)
		finalMessage = strings.ReplaceAll(finalMessage, "{count}", fmt.Sprintf("%d", count))
	} else {
		// Fallback for old configs that didn't use placeholders
		statsText := fmt.Sprintf("\n\n🔗 <b>Sizning referal havolangiz:</b>\n%s", link)
		finalMessage = adText + statsText
	}

	photoID, _ := db.GetSetting("referral_ad_photo_id")

	var sentMsg tgbotapi.Message
	var errSend error

	inlineKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Qo'shilish ↗️", link),
		),
	)

	if photoID != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(photoID))
		photoMsg.Caption = finalMessage
		photoMsg.ParseMode = "HTML"
		photoMsg.ReplyMarkup = inlineKb
		sentMsg, errSend = bot.Send(photoMsg)
	} else {
		txtMsg := tgbotapi.NewMessage(chatID, finalMessage)
		txtMsg.ParseMode = "HTML"
		txtMsg.ReplyMarkup = inlineKb
		sentMsg, errSend = bot.Send(txtMsg)
	}

	if errSend == nil {
		replyText := "👆 👆 👆 Bu postda sizning shaxsiy linkingiz joylashgan\n\nYuqoridagi postni yaqinlaringizga tarqating, ular sizning linkingizni bosib botga start berishi va telegram kanalga obuna bo'lishi kerak."
		replyMsg := tgbotapi.NewMessage(chatID, replyText)
		replyMsg.ReplyToMessageID = sentMsg.MessageID
		bot.Send(replyMsg)
	}
}

// handleAksiya shows the current promotion text
func handleAksiya(bot *tgbotapi.BotAPI, chatID int64) {
	text, err := db.GetSetting("aksiya_text")
	if err != nil || text == "" {
		text = "⚡️ Hozircha faol aksiyalar yo'q. Kuzatib boring!"
	}

	msg := tgbotapi.NewMessage(chatID, "🎁 <b>Aksiya & Takliflar</b>\n\n"+text)
	msg.ParseMode = "HTML"
	bot.Send(msg)
}

// ─── Helper ───────────────────────────────────────────────────────────

func send(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	bot.Send(msg)
}
