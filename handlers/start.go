package handlers

import (
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/db"
	"tanlov-bot/keyboards"
)

// HandleStart processes the /start command with optional referral parameter
func HandleStart(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, superAdminID int64) {
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

	// Fetch user to check phone
	user, err := db.GetUser(userID)
	if err != nil || user.Phone == "" {
		// Stop flow, ask for phone number
		msg := tgbotapi.NewMessage(chatID, "📱 <b>Ro'yxatdan o'tish uchun telefon raqamingizni yuboring.</b>\n\n<i>Pastdagi tugmani bosing:</i>")
		msg.ParseMode = "HTML"
		msg.ReplyMarkup = keyboards.RequestContactKeyboard()
		bot.Send(msg)
		return
	}

	// ── Subscription gate ──
	ok, missing, err := CheckUserSubscriptions(bot, userID, false)
	if err != nil {
		log.Printf("[start] sub check error for user %d: %v", userID, err)
	}
	if !ok {
		SendSubscriptionGate(bot, chatID, missing)
		return
	}

	// ── Send welcome ──
	sendWelcome(bot, chatID)
}

// formatUserIdentifier helper
func formatUserIdentifier(username, fullName string) string {
	if username != "" {
		return "@" + username
	}
	return fullName
}

// sendWelcome sends the configured start message (with optional video)
func sendWelcome(bot *tgbotapi.BotAPI, chatID int64) {
	text, _ := db.GetSetting("start_message")
	videoFileID, _ := db.GetSetting("start_video_file_id")

	if videoFileID != "" {
		// Send video first
		video := tgbotapi.NewVideo(chatID, tgbotapi.FileID(videoFileID))
		video.Caption = text
		video.ParseMode = "HTML"
		if _, err := bot.Send(video); err != nil {
			log.Printf("[start] failed to send video: %v", err)
			// Fallback to text
			sendTextWelcome(bot, chatID, text)
		}
	} else {
		sendTextWelcome(bot, chatID, text)
	}

	// Send main menu after welcome
	menuMsg := tgbotapi.NewMessage(chatID, "📋 <b>Asosiy menyu:</b>")
	menuMsg.ParseMode = "HTML"
	menuMsg.ReplyMarkup = getMenuForUser(chatID)
	bot.Send(menuMsg)
}

func sendTextWelcome(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
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
