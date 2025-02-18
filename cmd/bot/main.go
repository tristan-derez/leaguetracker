package main

import (
	"log"
	"os"
	"time"

	"github.com/tristan-derez/league-tracker/internal/bot"
	"github.com/tristan-derez/league-tracker/internal/config"
)

func init() {
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(new(timeWriter))
}

type timeWriter struct{}

func (tw *timeWriter) Write(p []byte) (n int, err error) {
	timestamp := time.Now().Add(2 * time.Hour).Format("2006/01/02 15:04:05 ")

	return os.Stdout.Write(append([]byte(timestamp), p...))
}

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
