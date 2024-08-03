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
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please provide a summoner name.",
			},
		})
		return
	}

	summonerName := strings.ToLower(options[0].StringValue())

	// todo: valide summoner's name with Riot API.
	if err := b.storage.AddSummoner(summonerName); err != nil {
        log.Printf("Error adding summoner: %v", err)
        respondWithError(s, i, "An error occurred while adding the summoner. Please try again later.")
        return
    }

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Now following summoner: %s", summonerName),
		},
	})
}

func (b *Bot) handleRemove(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
    if len(options) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Please provide a summoner name.",
			},
		})
		return
	}

	summonerName := strings.ToLower(options[0].StringValue())
    
	if err := b.storage.RemoveSummoner(summonerName); err != nil {
        log.Printf("Error removing summoner: %v", err)
        respondWithError(s, i, "An error occurred while removing the summoner. Please try again later.")
        return
    }

    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: fmt.Sprintf("Removed summoner: %s", summonerName),
        },
    })
}

func (b *Bot) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	summoners, err := b.storage.ListSummoners()
    if err != nil {
        log.Printf("Error listing summoners: %v", err)
        respondWithError(s, i, "An error occurred while retrieving the list of summoners. Please try again later.")
        return
    }

	var content string
	if len(summoners) == 0 {
		content = "No summoners are currently being followed"
	} else {
		content = "Followed summoners:\n" + strings.Join(summoners, "\n")
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