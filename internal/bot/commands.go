package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
    switch i.ApplicationCommandData().Name {
    case "add":
        b.handleAdd(s, i)
    case "remove":
        b.handleRemove(s, i)
    case "list":
        b.handleList(s, i)
	case "ping": 
		b.handlePing(s, i)
	}
}

func (b *Bot) handleAdd(s *discordgo.Session, i *discordgo.InteractionCreate) {
    options := i.ApplicationCommandData().Options
    if len(options) == 0 {
        respondWithError(s, i, "Please provide a summoner name.")
        return
    }

    summonerName := strings.ToLower(options[0].StringValue())
	// todo: check on RIOT api if user exists
    guildID := i.GuildID

    if err := b.storage.AddSummoner(guildID, summonerName); err != nil {
        log.Printf("Error adding summoner: %v", err)
        respondWithError(s, i, "An error occurred while adding the summoner. Please try again later.")
        return
    }

    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: fmt.Sprintf("Summoner '%s' is now being tracked in this server.", summonerName),
        },
    })
}

func (b *Bot) handleRemove(s *discordgo.Session, i *discordgo.InteractionCreate) {
    options := i.ApplicationCommandData().Options
    if len(options) == 0 {
        respondWithError(s, i, "Please provide a summoner name.")
        return
    }

    summonerName := strings.ToLower(options[0].StringValue())
    guildID := i.GuildID

    if err := b.storage.RemoveSummoner(guildID, summonerName); err != nil {
        log.Printf("Error removing summoner: %v", err)
        respondWithError(s, i, "An error occurred while removing the summoner. Please try again later.")
        return
    }

    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: fmt.Sprintf("Summoner '%s' has been removed from tracking in this server.", summonerName),
        },
    })
}

func (b *Bot) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
    guildID := i.GuildID

    summoners, err := b.storage.ListSummoners(guildID)
    if err != nil {
        log.Printf("Error listing summoners: %v", err)
        respondWithError(s, i, "An error occurred while retrieving the list of summoners. Please try again later.")
        return
    }

    var content string
    if len(summoners) == 0 {
        content = "No summoners are currently being tracked in this server."
    } else {
        content = "Tracked summoners in this server:\n"
        for _, s := range summoners {
            rank := "Unknown"
            if s.Rank != nil {
                rank = *s.Rank
            }
            leaguePoints := 0
            if s.LeaguePoints != nil {
                leaguePoints = *s.LeaguePoints
            }
            content += fmt.Sprintf("- %s (Rank: %s, LP: %d)\n", s.Name, rank, leaguePoints)
        }
    }

    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: content,
        },
    })
}

func (b *Bot) handlePing(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: "pong!",
        },
    })
}

func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: message,
            Flags:   discordgo.MessageFlagsEphemeral,
        },
    })
}