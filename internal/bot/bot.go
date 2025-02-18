package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/tristan-derez/league-tracker/internal/config"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
	"github.com/tristan-derez/league-tracker/internal/storage"
)

// Bot struct represents the Discord bot and holds references to its dependencies
type Bot struct {
	session    *discordgo.Session
	storage    *storage.Storage
	riotClient *riotapi.Client
	wg         sync.WaitGroup
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// New creates and initializes a new Bot instance
func New(cfg *config.Config) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, err
	}

	storage, err := storage.New(cfg)
	if err != nil {
		return nil, err
	}

	riotClient := riotapi.NewClient(cfg.RiotAPIKey, cfg.RiotAPIRegion)

	ctx, cancel := context.WithCancel(context.Background())

	bot := &Bot{
		session:    session,
		storage:    storage,
		riotClient: riotClient,
		ctx:        ctx,
		cancel:     cancel,
	}

	return bot, nil
}

// Run starts the bot and sets up event handlers
func (b *Bot) Run() error {
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Bot is now ready")
		if err := b.registerCommandsOnce(); err != nil {
			log.Printf("Failed to register commands: %v", err)
		}

		b.mu.Lock()
		for _, guild := range r.Guilds {
			if err := b.storage.AddGuild(guild.ID, guild.Name); err != nil {
				log.Printf("Error adding guild to database: %v", err)
			}
		}
		b.mu.Unlock()
		log.Println("Initial guild setup complete")

		go b.TrackMatches()
	})

	b.session.AddHandler(b.handleGuildCreate)
	b.session.AddHandler(b.handleInteraction)

	err := b.session.Open()
	if err != nil {
		return err
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	return b.Shutdown()
}

var commandsRegistered = false
var commandsMu sync.Mutex

// registerCommandsOnce registers Discord slash commands for the bot.
// It ensures commands are only registered once to avoid duplication.
func (b *Bot) registerCommandsOnce() error {
	commandsMu.Lock()
	defer commandsMu.Unlock()

	if commandsRegistered {
		return nil
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "add",
			Description: "Add one or more League of Legends summoners to the followed list",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "summoners",
					Description: "The summoner name(s) to add to the followed list (comma-separated for multiple)",
					Required:    true,
				},
			},
		},
		{
			Name:        "remove",
			Description: "Remove a League of Legends summoner from the followed list",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "summoners",
					Description: "The summoner names to remove from the followed list (comma-separated for multiple)",
					Required:    true,
				},
			},
		},
		{
			Name:        "reset",
			Description: "Remove all summoners from the followed list for this server",
		},
		{
			Name:        "unchannel",
			Description: "Remove the assigned channel for updates about matches of summoners",
		},
		{
			Name:        "list",
			Description: "List all followed summoners",
		},
	}

	for _, v := range commands {
		_, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", v)
		if err != nil {
			log.Printf("Cannot create '%v' command: %v", v.Name, err)
			return err
		}
	}

	commandsRegistered = true
	return nil
}

// handleGuildCreate is called when the bot joins a new Discord guild (server).
// It adds the guild to the database and starts tracking matches for it.
func (b *Bot) handleGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	err := b.storage.AddGuild(g.ID, g.Name)
	if err != nil {
		log.Printf("Error adding new guild to database: %v", err)
		return
	}

	log.Printf("Added new guild to database: %s (%s)", g.Name, g.ID)
}

// Shutdown gracefully stops the bot and closes all resources.
// Returns an error if closing the Discord session or storage fails.
func (b *Bot) Shutdown() error {
	log.Println("Shutting down...")
	b.cancel()
	b.wg.Wait()

	if err := b.session.Close(); err != nil {
		return fmt.Errorf("error closing Discord session: %w", err)
	}

	if err := b.storage.Close(); err != nil {
		return fmt.Errorf("error closing storage: %w", err)
	}

	return nil
}

// Close releases resources held by the bot.
// It closes the storage and Discord session.
// This method should be called when the bot is no longer needed.
// Returns an error if closing either the storage or Discord session fails.
func (b *Bot) Close() error {
	if err := b.storage.Close(); err != nil {
		return fmt.Errorf("error closing storage: %w", err)
	}
	if err := b.session.Close(); err != nil {
		return fmt.Errorf("error closing Discord session: %w", err)
	}

	return nil
}
