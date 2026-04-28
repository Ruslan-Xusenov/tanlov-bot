package keyboards

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ─────────────────── Main User Menu ──────────────────────

func MainMenu() tgbotapi.ReplyKeyboardMarkup {
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

// ─────────────────── Phone Number Request ──────────────────

func RequestContactKeyboard() tgbotapi.ReplyKeyboardMarkup {
	btn := tgbotapi.NewKeyboardButtonContact("📱 Raqamni yuborish")
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(btn),
	)
}

// ─────────────────── Admin Menu ──────────────────────────

func AdminMenu() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Statistika"),
			tgbotapi.NewKeyboardButton("📢 Kanallar"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("✉️ Xabar yuborish"),
			tgbotapi.NewKeyboardButton("📣 Reklama matni"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📝 Start xabari"),
			tgbotapi.NewKeyboardButton("👥 Adminlar"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🔙 Foydalanuvchi menyu"),
		),
	)
}

// ─────────────────── Cancel Button ───────────────────────

func CancelKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❌ Bekor qilish"),
		),
	)
}

// ─────────────────── Subscription Check ──────────────────

func CheckSubscriptionKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Tekshirish", "check_sub"),
		),
	)
}

// ─────────────────── Channel Management ──────────────────

func ChannelListKeyboard(channels []struct {
	ID   int64
	Name string
}) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, ch := range channels {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 "+ch.Name, fmt.Sprintf("del_ch_%d", ch.ID)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Kanal qo'shish", "add_channel"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// ─────────────────── Admin Management ────────────────────

func AdminListKeyboard(admins []struct {
	ID       int64
	Username string
	Name     string
}) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, a := range admins {
		label := a.Name
		if a.Username != "" {
			label = "@" + a.Username
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ "+label, fmt.Sprintf("del_admin_%d", a.ID)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Admin qo'shish", "add_admin"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
