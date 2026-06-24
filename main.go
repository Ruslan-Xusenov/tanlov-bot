package main

import (
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tanlov-bot/config"
	"tanlov-bot/db"
	"tanlov-bot/handlers"
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

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	log.Println("[main] Bot is running. Press Ctrl+C to stop.")

	for update := range updates {
		go func(u tgbotapi.Update) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[panic] recovered: %v", r)
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
		// Calculate next midnight
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
		duration := nextMidnight.Sub(now)

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
		} else {
			log.Println("[daily_job] No winner today (no referrals made).")
		}
	}
}