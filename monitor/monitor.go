package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	botToken = "8820039022:AAFU2zZ-V92OP8m9SiHBRv95J1bOms8Ifl4"
	chatID   = "8462446411"
)

type sendMessageReq struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func sendToTelegram(text string) {
	if botToken == "" || chatID == "" {
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	reqBody := sendMessageReq{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[monitor] json error: %v", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[monitor] http error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[monitor] unexpected status code: %d", resp.StatusCode)
	}
}

func Alert(msg string) {
	text := fmt.Sprintf("🚨 <b>XATOLIK / OGOHLANTIRISH</b>\n\n%s\n\n⏰ %s", msg, time.Now().In(tashkentZone()).Format("2006-01-02 15:04:05"))
	go sendToTelegram(text)
}

func SpamAlert(userID int64, username string) {
	text := fmt.Sprintf("⚠️ <b>SPAM HUJUM ANIQLANDI</b>\n\nFoydalanuvchi qisqa vaqt ichida juda ko'p xabar yubordi.\n👤 UserID: <code>%d</code>\n📛 Username: @%s\n\n⏰ %s", userID, username, time.Now().In(tashkentZone()).Format("2006-01-02 15:04:05"))
	go sendToTelegram(text)
}

func DailyReport(newUsers, activeUsers, totalUsers int) {
	text := fmt.Sprintf("📊 <b>KUNLIK HISOBOT (Soat 10:00)</b>\n\n👥 Yangi qo'shilganlar: <b>%d</b>\n⚡️ Faol bo'lganlar (24s): <b>%d</b>\n📈 Jami foydalanuvchilar: <b>%d</b>\n\nHammasi joyida, bot qotishsiz ishlamoqda! ✅", newUsers, activeUsers, totalUsers)
	go sendToTelegram(text)
}

func tashkentZone() *time.Location {
	loc, err := time.LoadLocation("Asia/Tashkent")
	if err != nil {
		return time.UTC
	}
	return loc
}
