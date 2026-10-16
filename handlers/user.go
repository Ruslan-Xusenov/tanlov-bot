package handlers

import (
	"fmt"
	"html"
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
	case "📘 Qo'llanma", "Qo`llanma", "📘 Qo‘llanma", "Qo'llanma", "Qo‘llanma":
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
	var daily, total int
	db.DB.QueryRow(`SELECT referral_count FROM users WHERE id = $1`, userID).Scan(&daily)
	db.DB.QueryRow(`SELECT total_referral_count FROM users WHERE id = $1`, userID).Scan(&total)

	text := fmt.Sprintf("🏡 SAODAT SHAHRI loyihasidan xonadon xarid qiling!\n"+
		"🪙 Kuniga atigi 166 ming so'mdan to'lab, o'z xonadoningiz egasiga aylaning!\n"+
		"━━━━━━━━━━━━━━\n"+
		"👥 Siz taklif qilgan foydalanuvchilar:\n"+
		"📅 Bugun: %d ta   🌐 Jami: %d ta", daily, total)
	send(bot, chatID, text)
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

	var sb strings.Builder
	if isDaily {
		var daily, total int
		db.DB.QueryRow(`SELECT referral_count FROM users WHERE id = $1`, userID).Scan(&daily)
		db.DB.QueryRow(`SELECT total_referral_count FROM users WHERE id = $1`, userID).Scan(&total)

		sb.WriteString("🏡 SAODAT SHAHRI loyihasidan xonadon xarid qiling!\n" +
			"🪙 Kuniga atigi 166 ming so'mdan to'lab, o'z xonadoningiz egasiga aylaning!\n" +
			"━━━━━━━━━━━━━━\n" +
			fmt.Sprintf("👥 Siz taklif qilgan foydalanuvchilar:\n📅 Bugun: %d ta   🌐 Jami: %d ta\n", daily, total) +
			"━━━━━━━━━━━━━━\n" +
			"📅 Kunlik reyting (Top 5):\n")
	} else {
		sb.WriteString("🏡 SAODAT SHAHRI loyihasidan xonadon xarid qiling!\n\n" +
			"🟣 Boshlang'ich to'lovsiz\n" +
			"🩷 Bitta pasport bilan\n" +
			"🟡 60 oyga, foiz va penyasiz\n\n" +
			"━━━━━━━━━━━━━━\n" +
			"🏆 Umumiy reyting (Top 5):\n")
	}

	if len(users) == 0 {
		sb.WriteString("Hali hech kim ro'yxatdan o'tmagan.\n")
	} else {
		medals := []string{"🥇", "🥈", "🥉", "4️⃣", "5️⃣"}
		for i, u := range users {
			medal := fmt.Sprintf("%d.", i+1)
			if i < len(medals) {
				medal = medals[i]
			}

			name := html.EscapeString(u.FullName)
			if name == "" && u.Username != "" {
				name = "@" + html.EscapeString(u.Username)
			}
			if name == "" {
				name = fmt.Sprintf("User#%d", u.ID)
			}

			score := u.ReferralCount
			if !isDaily {
				score = u.TotalReferralCount
			}

			if isAdmin && isDaily {
				sb.WriteString(fmt.Sprintf("%s %s — %d ta (Jami: %d ta)\n", medal, name, score, u.TotalReferralCount))
			} else {
				sb.WriteString(fmt.Sprintf("%s %s — %d ta\n", medal, name, score))
			}
		}
	}

	sb.WriteString("━━━━━━━━━━━━━━\n")

	// Motivational message logic
	rank, fifthScore, err := db.GetUserRank(userID, isDaily)
	if err == nil {
		if rank == -1 {
			sb.WriteString("💡 <i>Siz avvalgi kunlarda g'olib bo'lganingiz uchun bugungi kunlik reytingda ishtirok etmaysiz (boshqalarga imkoniyat berish maqsadida). Lekin siz qo'shayotgan odamlar umumiy reytingda hisoblanmoqda! Olg'a!</i>\n")
		} else if rank <= limit {
			sb.WriteString(fmt.Sprintf("📍 Sizning hozirgi o'rningiz: %d-o'rin (Top %d talikdasiz! 🎉)\n🚀 O'rningizni saqlab qolish uchun do'stlaringizni taklif qilishda davom eting! 🎁\n", rank, limit))
		} else {
			missing := fifthScore - getScore(userID, isDaily) + 1 // +1 to overtake or tie
			if missing <= 0 {
				missing = 1 // At least 1 to be safe
			}
			sb.WriteString(fmt.Sprintf("📍 Sizning hozirgi o'rningiz: %d-o'rin\n🚀 Top %d ga kirish uchun yana %d ta do'stingizni taklif qiling. Olg'a, sovg'alar sizni kutmoqda! 🎁\n", rank, limit, missing))
		}
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	bot.Send(msg)
}

func getScore(userID int64, isDaily bool) int {
	var count int
	if isDaily {
		db.DB.QueryRow(`SELECT referral_count FROM users WHERE id = $1`, userID).Scan(&count)
	} else {
		db.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE referred_by = $1 AND referral_status = 1 AND web_app_passed = 1`, userID).Scan(&count)
	}
	return count
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
	total := 0
	if user != nil {
		count = user.ReferralCount
		total = user.TotalReferralCount
	}

	// Get admin-configured ad text
	adText, _ := db.GetSetting("referral_ad_text")
	if adText == "" {
		adText = "🚀 Do'stingizni taklif qiling!\n\n👇 Pastdagi tugmani bosing:\n\n🔗 <b>Sizning referal havolangiz:</b>\n{link}\n\n👥 Siz bugun chaqirgan odamlar: <b>{count} ta</b>\n📊 Umumiy chaqirgan odamlar: <b>{total} ta</b>"
	}

	var finalMessage string
	if strings.Contains(adText, "{link}") || strings.Contains(adText, "{count}") || strings.Contains(adText, "{total}") {
		finalMessage = strings.ReplaceAll(adText, "{link}", link)
		finalMessage = strings.ReplaceAll(finalMessage, "{count}", fmt.Sprintf("%d", count))
		finalMessage = strings.ReplaceAll(finalMessage, "{total}", fmt.Sprintf("%d", total))
	} else {
		// Fallback for old configs that didn't use placeholders
		statsText := fmt.Sprintf("\n\n🔗 <b>Sizning referal havolangiz:</b>\n%s", link)
		finalMessage = adText + statsText
	}

	videoID, _ := db.GetSetting("referral_ad_video_id")
	photoID, _ := db.GetSetting("referral_ad_photo_id")

	var sentMsg tgbotapi.Message
	var errSend error

	inlineKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Qo'shilish ↗️", link),
		),
	)

	if videoID != "" {
		videoMsg := tgbotapi.NewVideo(chatID, tgbotapi.FileID(videoID))
		runes := []rune(finalMessage)
		if len(runes) > 1024 {
			cutoff := 1000
			str1000 := string(runes[:1000])
			if idx := strings.LastIndex(str1000, "\n"); idx > 500 {
				cutoff = len([]rune(str1000[:idx]))
			}
			videoMsg.Caption = string(runes[:cutoff])
			videoMsg.ParseMode = "HTML"
			sentMsg, errSend = bot.Send(videoMsg)
			if errSend == nil {
				txtMsg := tgbotapi.NewMessage(chatID, finalMessage)
				txtMsg.ParseMode = "HTML"
				txtMsg.ReplyMarkup = inlineKb
				sentMsg, errSend = bot.Send(txtMsg)
			}
		} else {
			videoMsg.Caption = finalMessage
			videoMsg.ParseMode = "HTML"
			videoMsg.ReplyMarkup = inlineKb
			sentMsg, errSend = bot.Send(videoMsg)
		}
		if errSend != nil {
			log.Printf("[user] Failed to send referral video: %v, falling back to text", errSend)
			txtMsg := tgbotapi.NewMessage(chatID, finalMessage)
			txtMsg.ParseMode = "HTML"
			txtMsg.ReplyMarkup = inlineKb
			sentMsg, errSend = bot.Send(txtMsg)
		}
	} else if photoID != "" {
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
