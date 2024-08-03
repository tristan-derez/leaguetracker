package bot

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/tristan-derez/league-tracker/internal/config"
	"github.com/tristan-derez/league-tracker/internal/storage"
)

type Bot struct {
	session *discordgo.Session
	storage *storage.Storage
}

func New(cfg *config.Config) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}

	storage, err := storage.New(cfg)
	if err != nil {
        return nil, err
    }

	bot := &Bot{
		session: session,
		storage: storage,
	}

	return bot, nil
}

func (b *Bot) Run() error {
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
        log.Println("Bot is now ready")
        if err := b.registerCommands(); err != nil {
            log.Printf("Failed to register commands: %v", err)
        }
    })

    err := b.session.Open()
    if err != nil {
        return err
    }
    defer b.session.Close()

    log.Println("Bot is now running. Press CTRL-C to exit.")
    sc := make(chan os.Signal, 1)
    signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
    <-sc

    return nil
}

func (b *Bot) registerCommands() error {
	if b.session == nil {
        return fmt.Errorf("session is nil")
    }
    if b.session.State == nil {
        return fmt.Errorf("session state is nil")
    }
    if b.session.State.User == nil {
        return fmt.Errorf("session state user is nil")
    }

	b.session.AddHandler(b.handleInteraction)

	commands := []*discordgo.ApplicationCommand{
		{
			Name: "add",
			Description: "Add a League of Legends summoner to the followed list",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type: discordgo.ApplicationCommandOptionString,
					Name: "summoner",
					Description: "The summoner name to add to the followed list",
					Required: true,
				},
			},
		},
		{
			Name: "remove",
			Description: "Remove a League of Legends summoner from the followed list",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type: discordgo.ApplicationCommandOptionString,
					Name: "summoner",
					Description: "The summoner name to remove from the followed list",
					Required: true,
				},
			},
		},
		{
            Name:        "list",
            Description: "List all followed summoners",
        },
		{
            Name:        "ping",
            Description: "Check if LeagueTracker is online",
        },
	}

	for _, v := range commands {
		cmd, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", v)
		if err != nil {
			log.Printf("Cannot create '%v' command: %v", v.Name, err)
			return err
		}
		log.Printf("Registered command: %s", cmd.Name)
	}

	return nil
}

func (b *Bot) Close() error {
    if err := b.storage.Close(); err != nil {
        return err
    }
    return b.session.Close()
}