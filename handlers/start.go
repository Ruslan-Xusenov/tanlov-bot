package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/db"
)

// HandleStart processes the /start command with optional referral parameter
func HandleStart(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, superAdminID int64, botUsername string) {
	userID := msg.From.ID
	username := msg.From.UserName
	fullName := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	chatID := msg.Chat.ID

	// Parse referral code from /start ref_XXXXX
	var referrerID int64
	if msg.CommandArguments() != "" {
		arg := msg.CommandArguments()
		if strings.HasPrefix(arg, "ref_") {
			idStr := strings.TrimPrefix(arg, "ref_")
			parsed, err := strconv.ParseInt(idStr, 10, 64)
			if err == nil && parsed != userID {
				referrerID = parsed
			}
		}
	}

	// Check if user already exists
	exists, err := db.UserExists(userID)
	if err != nil {
		log.Printf("[start] db error checking user %d: %v", userID, err)
		// Safe fallback: assume user exists so we don't accidentally credit a re-referral
		exists = true
	}

	isNewUser := !exists

	if referrerID > 0 && isNewUser {
		// Register user with referral as pending
		if err := db.CreateUserWithReferral(userID, username, fullName, referrerID); err != nil {
			log.Printf("[start] failed to create user with referral: %v", err)
		}
	} else {
		// Register or update user normally
		if err := db.UpsertUser(userID, username, fullName); err != nil {
			log.Printf("[start] failed to upsert user: %v", err)
		}
	}

	// Ensure super admin is always in admins table
	if userID == superAdminID {
		db.AddAdmin(userID, 0)
	}

	// ── Subscription gate ──
	ok, missing, err := CheckUserSubscriptions(bot, userID, false)
	if err != nil {
		log.Printf("[start] sub check error for user %d: %v", userID, err)
	}
	if !ok {
		kb := BuildSubscriptionKeyboard(missing)
		sendWelcome(bot, chatID, false, &kb)
		return
	}

	// ── Send welcome ──
	sendWelcome(bot, chatID, false, nil)

	// ── Complete Registration ──
	CompleteRegistrationFlow(bot, chatID, userID, username, fullName, botUsername)
}

// formatUserIdentifier helper
func formatUserIdentifier(username, fullName string) string {
	if username != "" {
		return "@" + username
	}
	return fullName
}

// sendWelcome sends the configured start message (with optional video)
func sendWelcome(bot *tgbotapi.BotAPI, chatID int64, sendMenu bool, inlineKb *tgbotapi.InlineKeyboardMarkup) {
	text, _ := db.GetSetting("start_message")
	videoFileID, _ := db.GetSetting("start_video_file_id")

	if videoFileID != "" {
		// Send video first
		video := tgbotapi.NewVideo(chatID, tgbotapi.FileID(videoFileID))
		video.Caption = text
		video.ParseMode = "HTML"
		if inlineKb != nil {
			video.ReplyMarkup = *inlineKb
		}
		if _, err := bot.Send(video); err != nil {
			log.Printf("[start] failed to send video: %v", err)
			// Fallback to text
			sendTextWelcome(bot, chatID, text, inlineKb)
		}
	} else {
		sendTextWelcome(bot, chatID, text, inlineKb)
	}

	if sendMenu {
		SendMenu(bot, chatID)
	}
}

func SendMenu(bot *tgbotapi.BotAPI, chatID int64) {
	menuMsg := tgbotapi.NewMessage(chatID, "📋 <b>Asosiy menyu:</b>")
	menuMsg.ParseMode = "HTML"
	menuMsg.ReplyMarkup = getMenuForUser(chatID)
	bot.Send(menuMsg)
}

// CompleteRegistrationFlow approves referrals and sends referral links
func CompleteRegistrationFlow(bot *tgbotapi.BotAPI, chatID, userID int64, username, fullName, botUsername string) {
	// 1. Try to approve referral
	user, err := db.GetUser(userID)
	if err == nil && user != nil && user.ReferralStatus == 0 && user.ReferredBy > 0 {
		if err := db.ApproveReferral(userID); err == nil {
			who := formatUserIdentifier(username, fullName)
			text := fmt.Sprintf("🎉 <b>Referalingiz tasdiqlandi!</b>\n\n👤 %s botdan ro'yxatdan o'tdi va sizga referal bali qo'shildi.", who)
			notifyMsg := tgbotapi.NewMessage(user.ReferredBy, text)
			notifyMsg.ParseMode = "HTML"
			bot.Send(notifyMsg)
		}
	}

	// 2. Send menu
	SendMenu(bot, chatID)

	// 3. Send Referral Link
	handleReferral(bot, chatID, userID, botUsername)

	// 4. Send Qullanma
	qullanmaText, _ := db.GetSetting("qullanma_text")
	if qullanmaText == "" {
		qullanmaText = "📄 <b>Qo'llanma</b>\nSizga berilgan referal havoladan nusxa oling va do'stlaringizga yuboring."
	}
	qMsg := tgbotapi.NewMessage(chatID, qullanmaText)
	qMsg.ParseMode = "HTML"
	bot.Send(qMsg)
}

func sendTextWelcome(bot *tgbotapi.BotAPI, chatID int64, text string, inlineKb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	if inlineKb != nil {
		msg.ReplyMarkup = *inlineKb
	}
	bot.Send(msg)
}

func getMenuForUser(userID int64) tgbotapi.ReplyKeyboardMarkup {
	if db.IsAdmin(userID) {
		return adminMenuKeyboard()
	}
	return userMenuKeyboard()
}

func userMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Reyting"),
			tgbotapi.NewKeyboardButton("🔗 Referal havolam"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🎁 Aksiya"),
		),
	)
}

func adminMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Reyting"),
			tgbotapi.NewKeyboardButton("🔗 Referal havolam"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🎁 Aksiya"),
			tgbotapi.NewKeyboardButton("⚙️ Admin panel"),
		),
	)
}
