package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/db"
)

type adminStateStore struct {
	mu    sync.RWMutex
	store map[int64]string
}

var adminState = &adminStateStore{store: make(map[int64]string)}

func (s *adminStateStore) Set(chatID int64, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[chatID] = state
}

func (s *adminStateStore) Get(chatID int64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store[chatID]
}

func (s *adminStateStore) Clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, chatID)
}

func HandleAdminPanel(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "⚙️ <b>Admin Panel</b>\n\nKerakli bo'limni tanlang:")
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = adminPanelKeyboard()
	bot.Send(msg)
}

func adminPanelKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📢 Kanallar"),
			tgbotapi.NewKeyboardButton("📊 Statistika"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("✏️ Start xabari"),
			tgbotapi.NewKeyboardButton("🎁 Aksiya matni"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📣 Reklama matni"),
			tgbotapi.NewKeyboardButton("✉️ Xabar yuborish"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("👥 Adminlar"),
			tgbotapi.NewKeyboardButton("🔙 Orqaga"),
		),
	)
}

func handleAdminStats(bot *tgbotapi.BotAPI, chatID int64) {
	stats, err := db.GetUserStats()
	if err != nil {
		log.Printf("[admin] stats error: %v", err)
		send(bot, chatID, "❌ Xatolik yuz berdi.")
		return
	}
	text := fmt.Sprintf(
		"📊 <b>Foydalanuvchilar statistikasi</b>\n\n"+
			"👥 Jami: <b>%d ta</b>\n"+
			"✅ Aktiv (30 kun): <b>%d ta</b>\n"+
			"😴 Noaktiv: <b>%d ta</b>",
		stats.Total, stats.Active, stats.Inactive,
	)
	send(bot, chatID, text)
}

func handleAdminChannels(bot *tgbotapi.BotAPI, chatID int64) {
	channels, err := db.GetAllChannels()
	if err != nil {
		send(bot, chatID, "❌ Kanallarni yuklashda xatolik.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📢 <b>Majburiy obuna kanallari</b>\n\n")

	var rows [][]tgbotapi.InlineKeyboardButton
	if len(channels) == 0 {
		sb.WriteString("Hozircha kanallar yo'q.\n")
	}
	for _, ch := range channels {
		status := "✅"
		if !ch.IsActive {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %s (<code>%s</code>)\n", status, ch.ChannelName, ch.ChannelID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"🗑 "+ch.ChannelName, fmt.Sprintf("del_ch_%d", ch.ID),
			),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Kanal qo'shish", "add_channel"),
	))

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

func handleAdminAdmins(bot *tgbotapi.BotAPI, chatID int64) {
	admins, err := db.GetAllAdmins()
	if err != nil {
		send(bot, chatID, "❌ Adminlarni yuklashda xatolik.")
		return
	}

	var sb strings.Builder
	sb.WriteString("👥 <b>Adminlar ro'yxati</b>\n\n")

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, a := range admins {
		label := a.FullName
		if a.Username != "" {
			label = "@" + a.Username
		}
		sb.WriteString(fmt.Sprintf("• %s (<code>%d</code>)\n", label, a.ID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"❌ "+label, fmt.Sprintf("del_admin_%d", a.ID),
			),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Admin qo'shish", "add_admin"),
	))

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

func handleAdminStartMsg(bot *tgbotapi.BotAPI, chatID int64) {
	cur, _ := db.GetSetting("start_message")
	videoID, _ := db.GetSetting("start_video_file_id")

	videoLine := "❌ Video yo'q"
	if videoID != "" {
		videoLine = "✅ Video bor"
	}

	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("✏️ <b>Start xabari sozlamalari</b>\n\n"+
			"📹 %s\n\n"+
			"📝 <b>Joriy matn:</b>\n%s\n\n"+
			"Quyidan amalni tanlang:", videoLine, cur),
	)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Matnni o'zgartirish", "edit_start_text"),
			tgbotapi.NewInlineKeyboardButtonData("🎬 Video o'zgartirish", "edit_start_video"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Videoni o'chirish", "del_start_video"),
		),
	)
	bot.Send(msg)
}

func handleAdminAksiyaEdit(bot *tgbotapi.BotAPI, chatID int64) {
	cur, _ := db.GetSetting("aksiya_text")
	adminState.Set(chatID, "await_aksiya_text")

	cancelKb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
	)
	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("🎁 <b>Joriy aksiya matni:</b>\n\n%s\n\n✏️ Yangi matnni yuboring:", cur),
	)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = cancelKb
	bot.Send(msg)
}

func handleAdminReferralAdEdit(bot *tgbotapi.BotAPI, chatID int64) {
	cur, _ := db.GetSetting("referral_ad_text")
	adminState.Set(chatID, "await_referral_ad_text")

	cancelKb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
	)
	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("📣 <b>Joriy reklama matni:</b>\n\n%s\n\n"+
			"✏️ Yangi reklama matnini yuboring.\n"+
			"<i>Bu matn foydalanuvchi referal havolasini so'raganda, 'Botga o'tish' tugmasi ustida ko'rinadi.</i>", cur),
	)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = cancelKb
	bot.Send(msg)
}

func handleAdminBroadcastStart(bot *tgbotapi.BotAPI, chatID int64) {
	adminState.Set(chatID, "await_broadcast_msg")
	cancelKb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
	)
	msg := tgbotapi.NewMessage(chatID, "✉️ <b>Xabar yuborish (Broadcast)</b>\n\nBarcha foydalanuvchilarga yubormoqchi bo'lgan xabarni yuboring (Rasm, video yoki matn):")
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = cancelKb
	bot.Send(msg)
}

func doBroadcast(bot *tgbotapi.BotAPI, adminChatID int64, originalMsg *tgbotapi.Message) {
	userIDs, err := db.GetAllActiveUserIDs()
	if err != nil {
		send(bot, adminChatID, "❌ Bazadan foydalanuvchilarni olishda xatolik yuz berdi.")
		return
	}

	total := len(userIDs)
	sent := 0
	failed := 0

	send(bot, adminChatID, fmt.Sprintf("⏳ Xabar yuborish boshlandi. Jami aktiv foydalanuvchilar: %d ta. Tugagach xabar beraman.", total))

	for i, uid := range userIDs {
		copyMsg := tgbotapi.NewCopyMessage(uid, originalMsg.Chat.ID, originalMsg.MessageID)
		copyMsg.ReplyMarkup = originalMsg.ReplyMarkup
		
		_, err := bot.Send(copyMsg)
		if err != nil {
			failed++
			errStr := err.Error()
			if strings.Contains(errStr, "Forbidden") || strings.Contains(errStr, "blocked") || strings.Contains(errStr, "deactivated") || strings.Contains(errStr, "not found") {
				db.DeactivateUser(uid)
			}
		} else {
			sent++
		}

		if i > 0 && i%20 == 0 {
			time.Sleep(1 * time.Second)
		}
	}

	send(bot, adminChatID, fmt.Sprintf("✅ <b>Xabar yuborish yakunlandi!</b>\n\nJo'natildi: %d ta\nYetib bormadi (Bloklaganlar): %d ta", sent, failed))
}

func HandleAdminMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, callerID int64) {
	chatID := msg.Chat.ID
	text := msg.Text

	if text == "❌ Bekor qilish" {
		adminState.Clear(chatID)
		send(bot, chatID, "🚫 Bekor qilindi.")
		HandleAdminPanel(bot, chatID)
		return
	}

	state := adminState.Get(chatID)
	if state != "" {
		handleAdminState(bot, msg, callerID, state)
		return
	}

	switch text {
	case "⚙️ Admin panel":
		HandleAdminPanel(bot, chatID)
	case "📢 Kanallar":
		handleAdminChannels(bot, chatID)
	case "📊 Statistika":
		handleAdminStats(bot, chatID)
	case "✏️ Start xabari":
		handleAdminStartMsg(bot, chatID)
	case "🎁 Aksiya matni":
		handleAdminAksiyaEdit(bot, chatID)
	case "📣 Reklama matni":
		handleAdminReferralAdEdit(bot, chatID)
	case "✉️ Xabar yuborish":
		handleAdminBroadcastStart(bot, chatID)
	case "👥 Adminlar":
		handleAdminAdmins(bot, chatID)
	case "🔙 Orqaga":
		adminState.Clear(chatID)
		menuMsg := tgbotapi.NewMessage(chatID, "📋 <b>Asosiy menyu:</b>")
		menuMsg.ParseMode = "HTML"
		menuMsg.ReplyMarkup = adminMenuKeyboard()
		bot.Send(menuMsg)
	}
}

func handleAdminState(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, callerID int64, state string) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	switch state {

	case "await_broadcast_msg":
		go doBroadcast(bot, chatID, msg)
		adminState.Clear(chatID)
		HandleAdminPanel(bot, chatID)

	case "await_aksiya_text":
		if err := db.SetSetting("aksiya_text", text); err != nil {
			send(bot, chatID, "❌ Saqlashda xatolik.")
		} else {
			send(bot, chatID, "✅ Aksiya matni yangilandi!")
		}
		adminState.Clear(chatID)
		HandleAdminPanel(bot, chatID)

	case "await_referral_ad_text":
		if err := db.SetSetting("referral_ad_text", text); err != nil {
			send(bot, chatID, "❌ Saqlashda xatolik.")
		} else {
			send(bot, chatID, "✅ Reklama matni yangilandi! Endi foydalanuvchilar referal havolasini so'raganda yangi matn ko'rinadi.")
		}
		adminState.Clear(chatID)
		HandleAdminPanel(bot, chatID)

	case "await_start_text":
		if err := db.SetSetting("start_message", text); err != nil {
			send(bot, chatID, "❌ Saqlashda xatolik.")
		} else {
			send(bot, chatID, "✅ Start xabari yangilandi!")
		}
		adminState.Clear(chatID)
		HandleAdminPanel(bot, chatID)

	case "await_start_video":
		var fileID string
		if msg.Video != nil {
			fileID = msg.Video.FileID
		} else if msg.Document != nil {
			fileID = msg.Document.FileID
		} else {
			send(bot, chatID, "⚠️ Iltimos, video yuboring (fayl yoki video sifatida).")
			return
		}
		if err := db.SetSetting("start_video_file_id", fileID); err != nil {
			send(bot, chatID, "❌ Saqlashda xatolik.")
		} else {
			send(bot, chatID, "✅ Start videosi yangilandi!")
		}
		adminState.Clear(chatID)
		HandleAdminPanel(bot, chatID)

	case "await_channel_id":
		if !strings.HasPrefix(text, "@") && !strings.HasPrefix(text, "-100") {
			send(bot, chatID, "⚠️ Kanal IDsi @ yoki -100 bilan boshlanishi kerak.\nMasalan: @mykanalim")
			return
		}
		adminState.Set(chatID, "await_channel_name:"+text)
		cancelKb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
		)
		m := tgbotapi.NewMessage(chatID, "📝 Endi kanal nomini kiriting (ko'rsatish uchun):")
		m.ReplyMarkup = cancelKb
		bot.Send(m)

	default:
		if strings.HasPrefix(state, "await_channel_name:") {
			channelID := strings.TrimPrefix(state, "await_channel_name:")
			channelName := text

			url := "https://t.me/" + strings.TrimPrefix(channelID, "@")

			if err := db.AddChannel(channelID, channelName, url); err != nil {
				send(bot, chatID, "❌ Kanal qo'shishda xatolik. Balki allaqachon mavjud.")
			} else {
				send(bot, chatID, fmt.Sprintf("✅ <b>%s</b> kanali qo'shildi!", channelName))
			}
			adminState.Clear(chatID)
			HandleAdminPanel(bot, chatID)
			return
		}

		if strings.HasPrefix(state, "await_add_admin:") {
			addAdminByInput(bot, chatID, text, callerID)
			adminState.Clear(chatID)
			HandleAdminPanel(bot, chatID)
			return
		}

		adminState.Clear(chatID)
	}
}

func HandleAdminCallback(bot *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery, callerID int64) {
	chatID := cq.Message.Chat.ID
	data := cq.Data

	bot.Request(tgbotapi.NewCallback(cq.ID, ""))

	switch {
	case data == "add_channel":
		adminState.Set(chatID, "await_channel_id")
		cancelKb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
		)
		m := tgbotapi.NewMessage(chatID, "📢 Kanal ID yoki username kiriting:\n<i>Masalan: @mykanalim yoki -1001234567890</i>")
		m.ParseMode = "HTML"
		m.ReplyMarkup = cancelKb
		bot.Send(m)

	case strings.HasPrefix(data, "del_ch_"):
		idStr := strings.TrimPrefix(data, "del_ch_")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil {
			db.RemoveChannel(id)
			send(bot, chatID, "✅ Kanal o'chirildi.")
		}
		handleAdminChannels(bot, chatID)

	case data == "add_admin":
		adminState.Set(chatID, "await_add_admin:")
		cancelKb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
		)
		m := tgbotapi.NewMessage(chatID,
			"👤 <b>Admin qo'shish</b>\n\nFoydalanuvchi Telegram ID (<code>123456789</code>) yoki "+
				"username (<code>@username</code>) ni kiriting:")
		m.ParseMode = "HTML"
		m.ReplyMarkup = cancelKb
		bot.Send(m)

	case strings.HasPrefix(data, "del_admin_"):
		idStr := strings.TrimPrefix(data, "del_admin_")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil && id != callerID {
			db.RemoveAdmin(id)
			send(bot, chatID, "✅ Admin o'chirildi.")
		} else if id == callerID {
			send(bot, chatID, "⚠️ O'zingizni adminlikdan o'chira olmaysiz.")
		}
		handleAdminAdmins(bot, chatID)

	case data == "edit_start_text":
		adminState.Set(chatID, "await_start_text")
		cancelKb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
		)
		m := tgbotapi.NewMessage(chatID, "✏️ Yangi start xabarini yuboring:")
		m.ReplyMarkup = cancelKb
		bot.Send(m)

	case data == "edit_start_video":
		adminState.Set(chatID, "await_start_video")
		cancelKb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("❌ Bekor qilish")),
		)
		m := tgbotapi.NewMessage(chatID, "🎬 Yangi video yuboring (video yoki fayl sifatida):")
		m.ReplyMarkup = cancelKb
		bot.Send(m)

	case data == "del_start_video":
		db.SetSetting("start_video_file_id", "")
		send(bot, chatID, "✅ Video o'chirildi.")
		handleAdminStartMsg(bot, chatID)
	}
}

func addAdminByInput(bot *tgbotapi.BotAPI, chatID int64, input string, addedBy int64) {
	input = strings.TrimSpace(input)

	var targetID int64
	var targetName string

	if strings.HasPrefix(input, "@") {
		username := strings.TrimPrefix(input, "@")
		user, err := db.GetUserByUsername(username)
		if err != nil || user == nil {
			send(bot, chatID,
				fmt.Sprintf("❌ <code>@%s</code> topilmadi. Avval botdan foydalanishi kerak.", username))
			return
		}
		targetID = user.ID
		targetName = input
	} else {
		id, err := strconv.ParseInt(input, 10, 64)
		if err != nil {
			send(bot, chatID, "❌ Noto'g'ri format. ID raqam yoki @username bo'lishi kerak.")
			return
		}
		targetID = id
		targetName = fmt.Sprintf("<code>%d</code>", id)
	}

	if err := db.AddAdmin(targetID, addedBy); err != nil {
		send(bot, chatID, "❌ Admin qo'shishda xatolik.")
		return
	}
	send(bot, chatID, fmt.Sprintf("✅ %s admin sifatida qo'shildi!", targetName))
}