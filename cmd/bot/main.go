package main

import (
	"log"

	"github.com/tristan-derez/league-tracker/internal/bot"
	"github.com/tristan-derez/league-tracker/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	if err := b.Run(); err != nil {
		log.Fatalf("Bot stopped: %v", err)
	}
}