package handlers

import (
	"log"

	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/db"
)

// Router holds shared state needed across all handlers
type Router struct {
	Bot          *tgbotapi.BotAPI
	SuperAdminID int64
	BotUsername  string
}

// Route dispatches an incoming update to the correct handler
func (r *Router) Route(update tgbotapi.Update) {
	// ── Inline Callback Query ────────────────────────────────────────
	if update.CallbackQuery != nil {
		cq := update.CallbackQuery
		userID := cq.From.ID
		chatID := cq.Message.Chat.ID

		db.TouchUserActivity(userID)

		if cq.Data == "check_sub" {
			ok, missing, err := CheckUserSubscriptions(r.Bot, userID, true)
			if err != nil {
				log.Printf("[router] sub check error: %v", err)
			}
			if ok {
				// Subscription passed
				// 1. Check if we need to award a pending referral
				user, err := db.GetUser(userID)
				if err == nil && user != nil && user.ReferralStatus == 0 && user.ReferredBy > 0 {
					if err := db.ApproveReferral(userID); err == nil {
						// Notify referrer
						who := formatUserIdentifier(cq.From.UserName, cq.From.FirstName+" "+cq.From.LastName)
						text := fmt.Sprintf("🎉 <b>Referalingiz tasdiqlandi!</b>\n\n👤 %s majburiy kanallarga obuna bo'ldi va sizga referal bali qo'shildi.", who)
						msg := tgbotapi.NewMessage(user.ReferredBy, text)
						msg.ParseMode = "HTML"
						r.Bot.Send(msg)
					}
				}

				// 2. Remove gate message and show menu
				r.Bot.Request(tgbotapi.NewDeleteMessage(chatID, cq.Message.MessageID))
				sendWelcome(r.Bot, chatID)
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

	db.TouchUserActivity(userID)

	isAdmin := db.IsAdmin(userID) || userID == r.SuperAdminID

	// ── Handle Contact (Phone number sharing) ──
	if msg.Contact != nil {
		if msg.Contact.UserID == userID {
			phone := msg.Contact.PhoneNumber
			if !(strings.HasPrefix(phone, "998") || strings.HasPrefix(phone, "+998")) {
				send(r.Bot, chatID, "⚠️ Iltimos, faqat O'zbekiston (+998) raqamidan ro'yxatdan o'ting.")
				return
			}

			if err := db.UpdateUserPhone(userID, phone); err != nil {
				log.Printf("[router] failed to save phone: %v", err)
			}
			
			// Remove the reply keyboard
			removeKb := tgbotapi.NewRemoveKeyboard(true)
			rmMsg := tgbotapi.NewMessage(chatID, "✅ Raqamingiz qabul qilindi!")
			rmMsg.ReplyMarkup = removeKb
			r.Bot.Send(rmMsg)

			// Proceed to subscription check
			ok, missing, err := CheckUserSubscriptions(r.Bot, userID, false)
			if err != nil || !ok {
				SendSubscriptionGate(r.Bot, chatID, missing)
				return
			}
			sendWelcome(r.Bot, chatID)
		} else {
			send(r.Bot, chatID, "⚠️ Iltimos, o'zingizning raqamingizni yuboring.")
		}
		return
	}

	// For any other text message, block if they don't have a phone yet
	user, err := db.GetUser(userID)
	if (err != nil || user.Phone == "") && msg.IsCommand() && msg.Command() == "start" {
		HandleStart(r.Bot, msg, r.SuperAdminID)
		return
	} else if err != nil || user.Phone == "" {
		send(r.Bot, chatID, "⚠️ Iltimos, avval /start ni bosing va telefon raqamingizni yuboring.")
		return
	}

	// /start command (handled before subscription gate)
	if msg.IsCommand() && msg.Command() == "start" {
		HandleStart(r.Bot, msg, r.SuperAdminID)
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
		msg.Text == "👥 Adminlar" || msg.Text == "✉️ Xabar yuborish" ||
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
