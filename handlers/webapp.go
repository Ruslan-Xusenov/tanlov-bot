package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/mojocn/base64Captcha"
	"tanlov-bot/config"
	"tanlov-bot/db"
	"tanlov-bot/keyboards"
)

var (
	webAppBot *tgbotapi.BotAPI
	webAppCfg *config.Config

	// Captcha store and driver for Web App
	webStore  = base64Captcha.DefaultMemStore
	webDriver = base64Captcha.NewDriverDigit(80, 240, 5, 0.7, 80)
)

// RegisterRequest is the JSON body from the Web App form
type RegisterRequest struct {
	UserID       int64  `json:"user_id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Region       string `json:"region"`
	Username     string `json:"username"`
	DeviceInfo   string `json:"device_info"`
	CaptchaID    string `json:"captcha_id"`
	CaptchaAnswer string `json:"captcha_answer"`
}

// RegisterResponse is the JSON response to the Web App
type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CaptchaResponse is the JSON response for captcha generation
type CaptchaResponse struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

// StartWebAppServer starts the HTTP server for the Web App
func StartWebAppServer(cfg *config.Config, bot *tgbotapi.BotAPI) {
	webAppBot = bot
	webAppCfg = cfg

	mux := http.NewServeMux()

	// Serve the Web App HTML
	mux.HandleFunc("/webapp", handleWebAppPage)

	// API endpoints
	mux.HandleFunc("/api/captcha", corsMiddleware(handleCaptchaGenerate))
	mux.HandleFunc("/api/register", corsMiddleware(handleRegister))

	addr := ":" + cfg.WebAppPort
	log.Printf("[webapp] HTTP server starting on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[webapp] HTTP server failed: %v", err)
	}
}

// corsMiddleware adds CORS headers for Telegram Web App
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// handleWebAppPage serves the HTML registration form
func handleWebAppPage(w http.ResponseWriter, r *http.Request) {
	htmlPath := "captcha/webapp.html"
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// handleCaptchaGenerate creates a new captcha and returns it as base64 image
func handleCaptchaGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "Faqat GET so'rovlar qabul qilinadi", http.StatusMethodNotAllowed)
		return
	}

	captcha := base64Captcha.NewCaptcha(webDriver, webStore)
	id, b64s, _, err := captcha.Generate()
	if err != nil {
		log.Printf("[webapp] captcha generate error: %v", err)
		jsonError(w, "Captcha yaratishda xatolik", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, CaptchaResponse{
		ID:    id,
		Image: b64s,
	})
}

// handleRegister processes the registration form submission
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "Faqat POST so'rovlar qabul qilinadi", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, RegisterResponse{Success: false, Message: "Noto'g'ri ma'lumot formati"})
		return
	}

	// Validate required fields
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Region = strings.TrimSpace(req.Region)

	if req.UserID == 0 || req.FirstName == "" || req.LastName == "" || req.Region == "" {
		jsonResponse(w, RegisterResponse{Success: false, Message: "Barcha maydonlarni to'ldiring"})
		return
	}

	// Validate captcha
	if req.CaptchaID == "" || req.CaptchaAnswer == "" {
		jsonResponse(w, RegisterResponse{Success: false, Message: "Captcha kodini kiriting"})
		return
	}

	if !webStore.Verify(req.CaptchaID, req.CaptchaAnswer, true) {
		jsonResponse(w, RegisterResponse{Success: false, Message: "Captcha kodi noto'g'ri! Qaytadan urinib ko'ring."})
		return
	}

	// Check if user already registered (has region)
	if db.IsUserRegistered(req.UserID) {
		jsonResponse(w, RegisterResponse{Success: false, Message: "Bu Telegram akkaunt allaqachon ro'yxatdan o'tgan!"})
		return
	}

	// Get client IP address
	clientIP := getClientIP(r)

	// Check if IP is a VPN/Proxy
	if isVPN, err := checkIPQuality(clientIP); err == nil && isVPN {
		jsonResponse(w, RegisterResponse{
			Success: false, 
			Message: "Siz VPN yoki Proxy tarmog'idan kirdingiz.\n\nIltimos, VPN'ni o'chirib, o'zingizning haqiqiy internetingiz orqali qayta urining!",
		})
		return
	}

	// Check IP address
	ipExists, _, existingName, err := db.CheckIPExists(clientIP, req.UserID)
	if err != nil {
		log.Printf("[webapp] IP check error: %v", err)
	}
	if ipExists {
		jsonResponse(w, RegisterResponse{
			Success: false,
			Message: fmt.Sprintf("Bu IP manzildan allaqachon ro'yxatdan o'tilgan!\n\nAvval ro'yxatdan o'tgan: %s\n\nHar bir qurilmadan faqat bitta akkaunt ro'yxatdan o'ta oladi!", existingName),
		})
		return
	}

	// Generate device ID from user-agent
	deviceID := createDeviceID(req.DeviceInfo, clientIP)

	// Check device ID
	deviceExists, _, existingDeviceName, err := db.CheckDeviceExists(deviceID, req.UserID)
	if err != nil {
		log.Printf("[webapp] device check error: %v", err)
	}
	if deviceExists {
		jsonResponse(w, RegisterResponse{
			Success: false,
			Message: fmt.Sprintf("Bu qurilmadan allaqachon ro'yxatdan o'tilgan!\n\nAvval ro'yxatdan o'tgan: %s\n\nHar bir qurilmadan faqat bitta akkaunt ro'yxatdan o'ta oladi!", existingDeviceName),
		})
		return
	}

	// Register the user
	if err := db.RegisterFromWebApp(req.UserID, req.FirstName, req.LastName, req.Region, req.Username, deviceID, clientIP); err != nil {
		log.Printf("[webapp] registration error: %v", err)
		jsonResponse(w, RegisterResponse{Success: false, Message: "Xatolik yuz berdi. Qayta urinib ko'ring."})
		return
	}

	// Send phone request via Telegram Bot
	sendPhoneRequestViaBot(req.UserID)

	jsonResponse(w, RegisterResponse{Success: true, Message: "Muvaffaqiyatli ro'yxatdan o'tdingiz!"})
}

// sendPhoneRequestViaBot sends a phone request message via the bot
func sendPhoneRequestViaBot(userID int64) {
	if webAppBot == nil {
		return
	}

	user, err := db.GetUser(userID)
	if err == nil && user != nil {
		if user.Phone != "" {
			// Already fully registered (since we only require 1 phone number now)
			menu := GetMenuForUser(userID)
			msg := tgbotapi.NewMessage(userID, "✅ <b>Muvaffaqiyatli ro'yxatdan o'tdingiz!</b>\n\nSiz allaqachon to'liq ro'yxatdan o'tgansiz.")
			msg.ParseMode = "HTML"
			msg.ReplyMarkup = menu
			webAppBot.Send(msg)
			
			// We need username and fullname. Since we don't have them in this context easily, we can get from db user.
			CompleteRegistrationFlow(webAppBot, userID, userID, user.Username, user.FullName, webAppBot.Self.UserName)
			return
		}
	}

	messageText := "✅ <b>Muvaffaqiyatli ro'yxatdan o'tdingiz!</b>\n\n📱 <b>Endi telefon raqamingizni yuboring</b>\n\nRo'yxatdan o'tishni yakunlash uchun quyidagi tugmani bosing:"

	msg := tgbotapi.NewMessage(userID, messageText)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboards.RequestContactKeyboard()

	if _, err := webAppBot.Send(msg); err != nil {
		log.Printf("[webapp] failed to send phone request to user %d: %v", userID, err)
	}
}

// getClientIP extracts the real client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for reverse proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP in the chain
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	addr := r.RemoteAddr
	// Strip port number
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		addr = addr[:idx]
	}
	return addr
}

// createDeviceID generates a SHA-256 hash from device info
func createDeviceID(deviceInfo, ip string) string {
	parts := []string{}

	// Extract key device characteristics
	if strings.Contains(deviceInfo, "Android") {
		parts = append(parts, "Android")
	} else if strings.Contains(deviceInfo, "iPhone") {
		parts = append(parts, "iPhone")
	} else if strings.Contains(deviceInfo, "iPad") {
		parts = append(parts, "iPad")
	}

	// Add the full user-agent for uniqueness
	parts = append(parts, deviceInfo)
	parts = append(parts, ip)

	data := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// jsonResponse sends a JSON response
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// jsonError sends a JSON error response
func jsonError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: message})
}

// SendWebAppButton sends the Web App registration button to the user
func SendWebAppButton(bot *tgbotapi.BotAPI, chatID int64) {
	webAppURL := webAppCfg.WebAppURL + "/webapp"

	msg := tgbotapi.NewMessage(chatID, "📝 <b>Ro'yxatdan o'tish</b>\n\nQuyidagi tugmani bosib, ma'lumotlaringizni to'ldiring:")
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.InlineKeyboardButton{
				Text:   "📝 Ro'yxatdan o'tish",
				WebApp: &tgbotapi.WebAppInfo{URL: webAppURL},
			},
		),
	)

	if _, err := bot.Send(msg); err != nil {
		log.Printf("[webapp] failed to send webapp button to %d: %v", chatID, err)
	}
}

// IP API response struct
type IPAPIResponse struct {
	Status  string `json:"status"`
	Proxy   bool   `json:"proxy"`
	Hosting bool   `json:"hosting"`
}

// checkIPQuality checks if the IP is a proxy or VPN
func checkIPQuality(ip string) (bool, error) {
	// Don't check local IPs
	if ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") {
		return false, nil
	}

	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,proxy,hosting", ip)
	
	// Create a client with timeout
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result IPAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	if result.Status == "success" {
		if result.Proxy || result.Hosting {
			return true, nil
		}
	}
	return false, nil
}