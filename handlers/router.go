package handlers

import (
	"log"

	"fmt"
	"strings"
	"sync"
	"time"

	"tanlov-bot/db"
	"tanlov-bot/monitor"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Router holds shared state needed across all handlers
type Router struct {
	Bot          *tgbotapi.BotAPI
	SuperAdminID int64
	BotUsername  string
}

var (
	spamCache   = make(map[int64][]time.Time)
	spamMu      sync.Mutex
	alertedSpam = make(map[int64]time.Time)
)

func isSpamming(userID int64) bool {
	spamMu.Lock()
	defer spamMu.Unlock()

	now := time.Now()
	times := spamCache[userID]

	valid := make([]time.Time, 0)
	for _, t := range times {
		if now.Sub(t) < 3*time.Second {
			valid = append(valid, t)
		}
	}
	valid = append(valid, now)
	spamCache[userID] = valid

	if len(valid) > 10 {
		// Alert only once per hour
		lastAlert, ok := alertedSpam[userID]
		if !ok || now.Sub(lastAlert) > time.Hour {
			alertedSpam[userID] = now
			return true // triggers alert in caller
		}
		return false // silently drop
	}
	return false
}

// Route dispatches an incoming update to the correct handler
func (r *Router) Route(update tgbotapi.Update) {
	// ── Inline Callback Query ────────────────────────────────────────
	if update.CallbackQuery != nil {
		cq := update.CallbackQuery
		userID := cq.From.ID
		chatID := cq.Message.Chat.ID

		if db.IsUserBanned(userID) {
			return
		}

		db.TouchUserActivity(userID)

		if cq.Data == "check_sub" {
			user, _ := db.GetUser(userID)
			if user != nil && user.Phone == "" && !HasPassedCaptcha(userID) {
				r.Bot.Request(tgbotapi.NewCallback(cq.ID, "⚠️ Oldin rasmdagi misolni yeching!"))
				return
			}
			
			ok, missing, err := CheckUserSubscriptions(r.Bot, userID, true)
			if err != nil {
				log.Printf("[router] sub check error: %v", err)
			}
			if ok {
				// Subscription passed
				r.Bot.Request(tgbotapi.NewDeleteMessage(chatID, cq.Message.MessageID))

				user, err := db.GetUser(userID)
				if err == nil && user != nil {
					if user.Phone == "" {
						SendPhoneRequest(r.Bot, chatID)
					} else if user.ExtraPhone == "" {
						send(r.Bot, chatID, "📞 Iltimos, doim foydalanadigan telefon raqamingizni yozma ravishda kiriting.")
					} else {
						CompleteRegistrationFlow(r.Bot, chatID, userID, cq.From.UserName, cq.From.FirstName+" "+cq.From.LastName, r.BotUsername)
					}
				}
			} else {
				// Edit the existing message to refresh channel list
				callback := tgbotapi.NewCallback(cq.ID, "⚠️ Hali ham obuna bo'lmadingiz!")
				r.Bot.Request(callback)
				SendSubscriptionGate(r.Bot, chatID, missing)
			}
			return
		}


		// Admin callbacks
		if db.IsAdmin(userID) || userID == r.SuperAdminID {
			HandleAdminCallback(r.Bot, cq, userID)
			return
		}

		r.Bot.Request(tgbotapi.NewCallback(cq.ID, "❌ Ruxsat yo'q"))
		return
	}

	// ── Chat Member Update (Leave detection) ─────────────────────────
	if update.ChatMember != nil {
		status := update.ChatMember.NewChatMember.Status
		if status == "left" || status == "kicked" {
			userID := update.ChatMember.NewChatMember.User.ID
			// If they leave a mandatory channel, revoke their active referral
			user, err := db.GetUser(userID)
			if err == nil && user != nil && user.ReferralStatus == 1 {
				// Verify if it's actually one of the mandatory channels
				channels, _ := db.GetActiveChannels()
				isMandatory := false
				for _, ch := range channels {
					if strings.TrimPrefix(ch.ChannelID, "@") == update.ChatMember.Chat.UserName || ch.ChannelID == fmt.Sprintf("%d", update.ChatMember.Chat.ID) || ch.ChannelID == fmt.Sprintf("-100%d", update.ChatMember.Chat.ID) {
						isMandatory = true
						break
					}
				}
				if isMandatory || len(channels) > 0 {
					if err := db.RevokeReferral(userID); err == nil {
						// Optional: Notify referrer they lost a point (can be irritating, maybe keep silent or uncomment)
						// msg := tgbotapi.NewMessage(user.ReferredBy, "📉 Sizning referalingiz kanallardan chiqib ketdi va sizdan bir referal ball ayirildi.")
						// r.Bot.Send(msg)
					}
				}
			}
		}
		return
	}

	// ── Regular Message ──
	if update.Message == nil {
		return
	}

	msg := update.Message
	userID := msg.From.ID
	chatID := msg.Chat.ID

	if isSpamming(userID) {
		monitor.SpamAlert(userID, msg.From.UserName)
		return // block request
	}

	if db.IsUserBanned(userID) {
		return // Ignore banned users entirely
	}

	db.TouchUserActivity(userID)

	isAdmin := db.IsAdmin(userID) || userID == r.SuperAdminID

	// ── Captcha Check ──
	if IsUserInCaptchaState(userID) {
		if msg.Text != "" {
			if CheckAndClearCaptcha(userID, msg.Text) {
				send(r.Bot, chatID, "✅ To'g'ri javob!")
				
				// After captcha passed, send them to the subscription gate
				ok, missing, err := CheckUserSubscriptions(r.Bot, userID, false)
				if err != nil {
					log.Printf("[router] sub check error for user %d: %v", userID, err)
				}
				if !ok {
					kb := BuildSubscriptionKeyboard(missing)
					sendWelcome(r.Bot, chatID, kb)
				} else {
					// They are already subscribed to everything
					sendWelcome(r.Bot, chatID, nil)
					SendPhoneRequest(r.Bot, chatID)
				}
			} else {
				send(r.Bot, chatID, "❌ Noto'g'ri javob. Qaytadan urinib ko'ring.")
				GenerateAndSendCaptcha(r.Bot, chatID, userID)
			}
		} else {
			send(r.Bot, chatID, "⚠️ Iltimos, rasmda ko'rsatilgan misolning javobini matn orqali yuboring.")
		}
		return
	}

	// ── Handle Contact (Phone number sharing) ──
	if msg.Contact != nil {
		if msg.Contact.UserID == userID {
			phone := msg.Contact.PhoneNumber
			if !(strings.HasPrefix(phone, "998") || strings.HasPrefix(phone, "+998")) {
				send(r.Bot, chatID, "⚠️ Iltimos, faqat O'zbekiston (+998) raqamidan ro'yxatdan o'ting.")
				return
			}

			// Check if phone already registered
			exists, _ := db.CheckPhoneExists(phone)
			if exists {
				send(r.Bot, chatID, "❌ Bu telefon raqami orqali allaqachon ro'yxatdan o'tilgan. Bitta raqamdan faqat bir marta foydalanish mumkin.")
				return
			}

			if err := db.UpdateUserPhone(userID, phone); err != nil {
				log.Printf("[router] failed to save phone: %v", err)
			}
			
			rmMsg := tgbotapi.NewMessage(chatID, "✅ Raqamingiz qabul qilindi!\n\n📞 Iltimos, doim foydalanadigan telefon raqamingizni kiriting.\n\nAgar sizga yoki siz taklif qilgan do‘stingizga yutuq chiqsa, g‘olibni tasdiqlash uchun siz bilan shaxsan bog‘lanamiz.")
			rmMsg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			r.Bot.Send(rmMsg)
		} else {
			send(r.Bot, chatID, "⚠️ Iltimos, o'zingizning raqamingizni yuboring. Boshqa profil raqami qabul qilinmaydi.")
		}
		return
	}

	// ── Phone Check Gate (Strictly require contact) ──
	user, err := db.GetUser(userID)
	if err == nil && user != nil {
		if user.Phone == "" {
			if !msg.IsCommand() {
				send(r.Bot, chatID, "⚠️ Iltimos, ro'yxatdan o'tish uchun \"☎️ Raqamni ulashish\" tugmasini bosing.")
				return
			}
		} else if user.ExtraPhone == "" {
			// They have phone, but no extra phone. Expecting text input.
			if !msg.IsCommand() && msg.Text != "" {
				text := strings.TrimSpace(msg.Text)
				// Remove common formatting characters
				cleaned := strings.ReplaceAll(text, " ", "")
				cleaned = strings.ReplaceAll(cleaned, "-", "")
				cleaned = strings.ReplaceAll(cleaned, "(", "")
				cleaned = strings.ReplaceAll(cleaned, ")", "")
				cleaned = strings.TrimPrefix(cleaned, "+")

				// Validate: must be digits only after cleaning
				if !isNumeric(cleaned) || len(cleaned) < 9 {
					send(r.Bot, chatID, "⚠️ Iltimos, haqiqiy telefon raqamingizni kiriting.\n\nMasalan: 901234567 yoki 998901234567")
					return
				}

				// Normalize to 998XXXXXXXXX format
				if len(cleaned) == 9 {
					cleaned = "998" + cleaned
				}
				if !strings.HasPrefix(cleaned, "998") || len(cleaned) != 12 {
					send(r.Bot, chatID, "⚠️ Faqat O'zbekiston (+998) raqamini kiriting.\n\nMasalan: 901234567 yoki 998901234567")
					return
				}

				// Check it's not the same as primary phone
				primaryCleaned := strings.ReplaceAll(user.Phone, "+", "")
				primaryCleaned = strings.ReplaceAll(primaryCleaned, " ", "")
				if cleaned == primaryCleaned {
					send(r.Bot, chatID, "⚠️ Bu raqam sizning asosiy raqamingiz bilan bir xil. Iltimos, boshqa raqam kiriting.")
					return
				}

				// Re-verify channel subscription before approving
				subOk, missing, _ := CheckUserSubscriptions(r.Bot, userID, false)
				if !subOk {
					kb := BuildSubscriptionKeyboard(missing)
					sendWelcome(r.Bot, chatID, kb)
					return
				}

				db.UpdateUserExtraPhone(userID, cleaned)
				send(r.Bot, chatID, "✅ Qo'shimcha raqam qabul qilindi!")
				CompleteRegistrationFlow(r.Bot, chatID, userID, msg.From.UserName, msg.From.FirstName+" "+msg.From.LastName, r.BotUsername)
			} else {
				send(r.Bot, chatID, "📞 Iltimos, doim foydalanadigan telefon raqamingizni yozma ravishda kiriting.\n\nMasalan: 901234567")
			}
			return
		}
	}

	// /start command (handled before subscription gate)
	if msg.IsCommand() && msg.Command() == "start" {
		HandleStart(r.Bot, msg, r.SuperAdminID, r.BotUsername)
		return
	}

	// Admin shortcut /admin
	if msg.IsCommand() && msg.Command() == "admin" {
		if !isAdmin {
			send(r.Bot, chatID, "❌ Sizda admin huquqi yo'q.")
			return
		}
		HandleAdminPanel(r.Bot, chatID)
		return
	}

	// Admin panel button from keyboard
	if msg.Text == "⚙️ Admin panel" || msg.Text == "📢 Kanallar" ||
		msg.Text == "📊 Statistika" || msg.Text == "✏️ Start xabari" ||
		msg.Text == "🎁 Aksiya matni" || msg.Text == "📣 Reklama matni" ||
		msg.Text == "📝 Qo'llanma matni" || msg.Text == "👥 Adminlar" ||
		msg.Text == "✉️ Xabar yuborish" ||
		msg.Text == "📥 Excel yuklab olish" ||
		msg.Text == "🔙 Orqaga" ||
		msg.Text == "❌ Bekor qilish" {

		if !isAdmin {
			send(r.Bot, chatID, "❌ Sizda admin huquqi yo'q.")
			return
		}
		HandleAdminMessage(r.Bot, msg, userID)
		return
	}

	// Check if admin is in an input state (multi-step flows)
	if isAdmin {
		state := adminState.Get(chatID)
		if state != "" {
			HandleAdminMessage(r.Bot, msg, userID)
			return
		}
	}

	// All other messages — route to user handlers
	// Admin can also access user features
	HandleUserMessage(r.Bot, msg, r.BotUsername)
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
