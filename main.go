package main

import (
	"log"

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

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	log.Println("[main] Bot is running. 按 Ctrl+C to stop.")

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