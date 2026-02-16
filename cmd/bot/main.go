package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"test-lerngruppen/internal/config"
	"test-lerngruppen/internal/discord"
	"test-lerngruppen/internal/store"
)

func main() {
	log.Println("Starting")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	st, err := store.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer st.Close()

	bot, err := discord.NewBot(cfg, st)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	if err := bot.Start(); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}
	defer func(bot *discord.Bot) {
		err := bot.Stop()
		if err != nil {
			log.Printf("Failed to stop bot: %v", err)
		}
	}(bot)

	log.Println("Bot is Online!")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
