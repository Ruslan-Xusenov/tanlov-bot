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
	case "📅 Kunlik reyting":
		handleRatingSelection(bot, chatID, userID, true, db.IsAdmin(userID))
	case "🌐 Umumiy reyting":
		handleRatingSelection(bot, chatID, userID, false, db.IsAdmin(userID))
	case "🔗 Taklif havolam":
		handleReferral(bot, chatID, userID, botUsername)
	case "🎁 Aksiya haqida":
		handleAksiya(bot, chatID)
	case "📘 Qo'llanma", "Qo`llanma": // Fallback for old buttons if any
		handleQullanma(bot, chatID, nil)
	case "💎 Ballarim":
		handleBallarim(bot, chatID, userID)
	}
}

func handleQullanma(bot *tgbotapi.BotAPI, chatID int64, markup interface{}) {
	qullanmaText, _ := db.GetSetting("qullanma_text")
	if qullanmaText == "" {
		qullanmaText = "📄 <b>Qo'llanma</b>\nSizga berilgan referal havoladan nusxa oling va do'stlaringizga yuboring."
	}
	
	qullanmaVideo, _ := db.GetSetting("qullanma_video_id")

	if qullanmaVideo != "" {
		vMsg := tgbotapi.NewVideo(chatID, tgbotapi.FileID(qullanmaVideo))
		
		// Telegram limits caption to 1024 chars.
		if len(qullanmaText) > 1000 {
			vMsg.Caption = "📄 <b>Qo'llanma</b>"
			vMsg.ParseMode = "HTML"
			bot.Send(vMsg)
			
			msg := tgbotapi.NewMessage(chatID, qullanmaText)
			msg.ParseMode = "HTML"
			if markup != nil {
				msg.ReplyMarkup = markup
			}
			bot.Send(msg)
		} else {
			vMsg.Caption = qullanmaText
			vMsg.ParseMode = "HTML"
			if markup != nil {
				vMsg.ReplyMarkup = markup
			}
			if _, err := bot.Send(vMsg); err != nil {
				log.Printf("[user] Failed to send Qullanma video: %v", err)
				msg := tgbotapi.NewMessage(chatID, qullanmaText)
				msg.ParseMode = "HTML"
				if markup != nil {
					msg.ReplyMarkup = markup
				}
				bot.Send(msg)
			}
		}
	} else {
		msg := tgbotapi.NewMessage(chatID, qullanmaText)
		msg.ParseMode = "HTML"
		if markup != nil {
			msg.ReplyMarkup = markup
		}
		bot.Send(msg)
	}
}

func handleBallarim(bot *tgbotapi.BotAPI, chatID, userID int64) {
	user, err := db.GetUser(userID)
	daily := 0
	total := 0
	if err == nil && user != nil {
		daily = user.ReferralCount
		total = user.TotalReferralCount
	}
	send(bot, chatID, fmt.Sprintf("👥 Siz chaqirgan foydalanuvchilar:\n\n📅 Bugun: <b>%d ta</b>\n🌐 Jami: <b>%d ta</b>", daily, total))
}


func handleRatingSelection(bot *tgbotapi.BotAPI, chatID int64, userID int64, isDaily bool, isAdmin bool) {
	var users []db.User
	var err error

	limit := 5 // We show top 5 as requested

	if isDaily {
		users, err = db.GetTopReferrersDaily(limit)
	} else {
		users, err = db.GetTopReferrersTotal(limit)
	}

	if err != nil {
		log.Printf("[user] failed to get top referrers: %v", err)
		send(bot, chatID, "❌ Xatolik yuz berdi. Iltimos, keyinroq urinib ko'ring.")
		return
	}

	title := "📅 <b>Kunlik Reyting (Top 5):</b>\n\n"
	if !isDaily {
		title = "🌐 <b>Umumiy Reyting (Top 5):</b>\n\n"
	}

	if len(users) == 0 {
		send(bot, chatID, title+"Hali hech kim ro'yxatdan o'tmagan.")
		return
	}

	medals := []string{"🥇", "🥈", "🥉"}
	var sb strings.Builder
	sb.WriteString(title)

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

		score := u.ReferralCount
		if !isDaily {
			score = u.TotalReferralCount
		}

		if isAdmin {
			sb.WriteString(fmt.Sprintf("%s <b>%s</b> — %d ta (Jami: %d ta)\n", medal, name, score, u.TotalReferralCount))
		} else {
			sb.WriteString(fmt.Sprintf("%s <b>%s</b> — %d ta\n", medal, name, score))
		}
	}

	// Motivational message logic
	if !isAdmin {
		rank, fifthScore, err := db.GetUserRank(userID, isDaily)
		if err == nil && rank > limit {
			missing := fifthScore - getScore(userID, isDaily) + 1 // +1 to overtake or tie
			if missing <= 0 {
				missing = 1 // At least 1 to be safe
			}
			sb.WriteString(fmt.Sprintf("\n💡 <i>Sizning hozirgi o'rningiz: <b>%d-o'rin</b>.\nTop %d reytingga kirishingiz uchun yana <b>%d ta</b> do'stingizni chaqirishingiz kerak! Olg'a!</i>", rank, limit, missing))
		}
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	bot.Send(msg)
}

func getScore(userID int64, isDaily bool) int {
	user, err := db.GetUser(userID)
	if err != nil || user == nil {
		return 0
	}
	if isDaily {
		return user.ReferralCount
	}
	return user.TotalReferralCount
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
		count = user.TotalReferralCount
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
		// Fallback to local picture.jpg
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath("picture.jpg"))
		photoMsg.Caption = finalMessage
		photoMsg.ParseMode = "HTML"
		photoMsg.ReplyMarkup = inlineKb
		sentMsg, errSend = bot.Send(photoMsg)
		
		// If picture.jpg fails (e.g. not found), send text only
		if errSend != nil {
			txtMsg := tgbotapi.NewMessage(chatID, finalMessage)
			txtMsg.ParseMode = "HTML"
			txtMsg.ReplyMarkup = inlineKb
			sentMsg, errSend = bot.Send(txtMsg)
		}
	}

	if errSend == nil {
		replyText := "👆 👆 👆 Bu postda sizning shaxsiy linkingiz joylashgan\n\nYuqoridagi postni yaqinlaringizga tarqating, ular sizning linkingizni bosib botga start berishi va telegram kanalga obuna bo'lishi kerak."
		replyMsg := tgbotapi.NewMessage(chatID, replyText)
		replyMsg.ReplyToMessageID = sentMsg.MessageID
		bot.Send(replyMsg)
	}
}

func handleAksiya(bot *tgbotapi.BotAPI, chatID int64) {
	text, _ := db.GetSetting("aksiya_text")
	photoID, _ := db.GetSetting("aksiya_photo_id")
	videoID, _ := db.GetSetting("aksiya_video_id")

	if text == "" && photoID == "" && videoID == "" {
		text = "⚡️ Hozircha faol aksiyalar yo'q. Kuzatib boring!"
	}

	caption := "🎁 <b>Aksiya & Takliflar</b>\n\n" + text
	if text == "" {
		caption = "🎁 <b>Aksiya & Takliflar</b>"
	}

	if videoID != "" {
		msg := tgbotapi.NewVideo(chatID, tgbotapi.FileID(videoID))
		msg.Caption = caption
		msg.ParseMode = "HTML"
		bot.Send(msg)
	} else if photoID != "" {
		msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(photoID))
		msg.Caption = caption
		msg.ParseMode = "HTML"
		bot.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(chatID, caption)
		msg.ParseMode = "HTML"
		bot.Send(msg)
	}
}

// ─── Helper ───────────────────────────────────────────────────────────

func send(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	bot.Send(msg)
}
