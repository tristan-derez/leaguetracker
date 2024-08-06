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

    summonerName := strings.TrimSpace(strings.ToLower(options[0].StringValue()))
    parts := strings.SplitN(summonerName, "#", 2)

    if len(parts) != 2 {
        log.Printf("Invalid summoner name format: %s", summonerName)
        respondWithError(s, i, "Invalid summoner name format. Please use Name#Tag.")
        return
    }

    gameName := strings.TrimSpace(parts[0])
    tagLine := strings.TrimSpace(parts[1])

    account, err := b.riotClient.GetAccountPUUIDBySummonerName(gameName, tagLine)
    if err != nil {
        log.Printf("Error getting summoner info: %v", err)
        respondWithError(s, i, "Unable to find summoner.")
        return
    }

    summoner, err := b.riotClient.GetSummonerByPUUID(account.SummonerPUUID)
    if err != nil {
        log.Printf("Error getting summoner info: %v", err)
        respondWithError(s, i, "Unable to find summoner.")
        return
    }

    guildID := i.GuildID

    rankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
    if err != nil {
        log.Printf("Error fetching summoner rank data: %v", err)
        respondWithError(s, i, fmt.Sprintf("Error fetching summoner rank data: %v", err))
        return
    }

    lastMatchData, err := b.riotClient.GetLastMatchData(account.SummonerPUUID)
    if err != nil {
        log.Printf("Error fetching last match data: %v", err)
    }

    if err := b.storage.AddSummoner(guildID, summonerName, *summoner, rankInfo); err != nil {
        log.Printf("Error adding summoner to database: %v", err)
        respondWithError(s, i, fmt.Sprintf("Error adding summoner to database: %v", err))
        return
    }

    if lastMatchData != nil {
        if err := b.storage.AddMatch(summoner.RiotSummonerID, lastMatchData); err != nil {
            log.Printf("Error adding match to database: %v", err)
        }
    }

    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Content: fmt.Sprintf("Summoner '%s' (Level %d) is now being tracked in this server.", summonerName, summoner.SummonerLevel),
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
        for _, summoner := range summoners {
            content += fmt.Sprintf("- %s (Rank: %s, LP: %d)\n", summoner.Name, summoner.Rank, summoner.LeaguePoints)
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