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
	// Get the current time and add 2 hours
	t := time.Now().Add(2 * time.Hour)

	// Format the time as desired
	timeStr := t.Format("2006/01/02 15:04:05 ")

	// Write the time and the log message
	return os.Stdout.Write(append([]byte(timeStr), p...))
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
