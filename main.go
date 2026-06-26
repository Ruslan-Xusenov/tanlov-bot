package main

import (
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/config"
	"tanlov-bot/db"
	"tanlov-bot/handlers"
	"tanlov-bot/monitor"
)

func main() {
	cfg := config.Load()

	db.Init(cfg.DatabaseURL)

	db.AddAdmin(cfg.SuperAdminID, 0)

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatalf("[main] failed to create bot: %v", err)
	}
	bot.Debug = false
	log.Printf("[main] Bot started: @%s", bot.Self.UserName)

	router := &handlers.Router{
		Bot:          bot,
		SuperAdminID: cfg.SuperAdminID,
		BotUsername:  bot.Self.UserName,
	}

	go runDailyRewardJob(bot)
	go runDailyReportJob()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	log.Println("[main] Bot is running. Press Ctrl+C to stop.")

	for update := range updates {
		go func(u tgbotapi.Update) {
			defer func() {
				if r := recover(); r != nil {
					errMsg := fmt.Sprintf("Panic yuz berdi: %v", r)
					log.Printf("[panic] %s", errMsg)
					monitor.Alert(errMsg)
				}
			}()
			router.Route(u)
		}(update)
	}

	log.Println("[main] Bot stopped.")
}

func runDailyRewardJob(bot *tgbotapi.BotAPI) {
	loc, err := time.LoadLocation("Asia/Tashkent")
	if err != nil {
		log.Printf("[daily_job] failed to load Tashkent timezone, using local: %v", err)
		loc = time.Local
	}

	for {
		now := time.Now().In(loc)
		// Calculate next 20:30
		target := time.Date(now.Year(), now.Month(), now.Day(), 20, 30, 0, 0, loc)
		if !now.Before(target) {
			target = target.AddDate(0, 0, 1) // Next day at 20:30
		}
		duration := target.Sub(now)

		log.Printf("[daily_job] Next reward execution in %v", duration)
		time.Sleep(duration)

		log.Println("[daily_job] Running daily reward process...")
		winner, err := db.ProcessDailyReward()
		if err != nil {
			log.Printf("[daily_job] error processing daily reward: %v", err)
			continue
		}

		if winner != nil {
			log.Printf("[daily_job] Daily winner is UserID: %d (%s) with %d referrals", winner.ID, winner.FullName, winner.ReferralCount)
			
			// Send message to winner
			msgText := fmt.Sprintf("🏆 <b>Tabriklaymiz!</b>\n\nSiz bugun eng ko'p (<b>%d ta</b>) do'stingizni taklif qilib, kunlik g'olib bo'ldingiz!", winner.ReferralCount)
			msg := tgbotapi.NewMessage(winner.ID, msgText)
			msg.ParseMode = "HTML"
			_, err = bot.Send(msg)
			if err != nil {
				log.Printf("[daily_job] failed to send winner message: %v", err)
			}

			// Notify admins
			admins, err := db.GetAllAdmins()
			if err == nil {
				adminText := fmt.Sprintf("🏆 <b>Bugungi g'olib aniqlandi!</b>\n\n"+
					"👤 <b>Ismi:</b> %s\n"+
					"🔗 <b>Username:</b> @%s\n"+
					"📱 <b>Raqami:</b> %s\n"+
					"🆔 <b>ID:</b> <code>%d</code>\n"+
					"👥 <b>Kunlik takliflari:</b> %d ta\n"+
					"📊 <b>Umumiy takliflari:</b> %d ta",
					winner.FullName, winner.Username, winner.Phone, winner.ID, winner.ReferralCount, winner.TotalReferralCount)

				for _, admin := range admins {
					adminMsg := tgbotapi.NewMessage(admin.ID, adminText)
					adminMsg.ParseMode = "HTML"
					bot.Send(adminMsg)
				}
			}
		} else {
			log.Println("[daily_job] No winner today (no referrals made).")
		}
	}
}

func runDailyReportJob() {
	loc, err := time.LoadLocation("Asia/Tashkent")
	if err != nil {
		loc = time.Local
	}

	for {
		now := time.Now().In(loc)
		
		// Target 10:00 AM
		target := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc)
		if now.After(target) {
			// If it's already past 10:00 AM today, schedule for tomorrow
			target = target.AddDate(0, 0, 1)
		}
		
		duration := target.Sub(now)
		time.Sleep(duration)
		
		newU, activeU, totalU := db.GetBotStats()
		monitor.DailyReport(newU, activeU, totalU)
	}
}