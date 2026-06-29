package handlers

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/mojocn/base64Captcha"
	"tanlov-bot/db"
)

var (
	// store holds the captchas in memory (auto-eviction)
	store = base64Captcha.DefaultMemStore
	// driver for distorted digit captchas
	driver = base64Captcha.NewDriverDigit(80, 240, 5, 0.7, 80)
	
	// userCaptchaState stores userID -> captchaID
	userCaptchaState sync.Map
	
	// passedCaptchaCache stores userID -> bool
	passedCaptchaCache sync.Map
)

// GenerateCaptcha sends a new math captcha to the user
func GenerateAndSendCaptcha(bot *tgbotapi.BotAPI, chatID int64, userID int64) error {
	captcha := base64Captcha.NewCaptcha(driver, store)
	
	id, b64s, _, err := captcha.Generate()
	if err != nil {
		return err
	}
	
	// Save state
	userCaptchaState.Store(userID, id)
	
	// b64s is data:image/png;base64,xxxx
	idx := strings.Index(b64s, ",")
	if idx < 0 {
		return fmt.Errorf("invalid base64 image generated")
	}
	rawB64 := b64s[idx+1:]
	
	imgBytes, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return err
	}
	
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "captcha.png",
		Bytes: imgBytes,
	})
	photo.Caption = "🤖 <b>Haqiqiy foydalanuvchi ekanligingizni tasdiqlang</b>\n\nSo‘nggi vaqtlarda ayrim foydalanuvchilar soxta akkauntlar orqali botdagi reytingini sun’iy oshirishga urinmoqda. Shu sababli tizimni tozalash jarayoni ketmoqda.\n\n📷 Iltimos, yuqoridagi rasmda ko'rib turgan <b>5 ta raqamni</b> yozib yuboring.\n\n⚠️ Faqat rasm ko'rsatilgan raqamlarni yozing. Boshqa so'z qo'shmang."
	photo.ParseMode = tgbotapi.ModeHTML
	
	_, err = bot.Send(photo)
	return err
}

// CheckAndClearCaptcha verifies the answer. Returns true if correct.
func CheckAndClearCaptcha(userID int64, answer string) bool {
	val, ok := userCaptchaState.Load(userID)
	if !ok {
		return false
	}
	captchaID := val.(string)
	
	// Verify removes the captcha from store automatically if correct (or according to its internal logic, actually DefaultMemStore removes it on Verify whether correct or not if clear=true).
	// We pass clear=true so it gets deleted.
	if store.Verify(captchaID, answer, true) {
		userCaptchaState.Delete(userID)
		passedCaptchaCache.Store(userID, true)
		db.SetCaptchaPassed(userID)
		return true
	}
	
	return false
}

// IsUserInCaptchaState returns true if user is currently expected to answer a captcha
func IsUserInCaptchaState(userID int64) bool {
	_, ok := userCaptchaState.Load(userID)
	return ok
}

// HasPassedCaptcha returns true if the user has successfully solved a captcha in this session.
// For fully registered users (with phone), we don't even check this (we can just allow them).
func HasPassedCaptcha(userID int64) bool {
	// First check database
	user, err := db.GetUser(userID)
	if err == nil && user != nil && user.CaptchaPassed == 1 {
		return true
	}
	// Fallback to in-memory cache
	_, ok := passedCaptchaCache.Load(userID)
	return ok
}
