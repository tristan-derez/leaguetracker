package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/tristan-derez/league-tracker/internal/utils"
)

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "add":
		b.handleAdd(s, i)
	case "remove":
		b.handleRemove(s, i)
	case "list":
		b.handleList(s, i)
	case "unchannel":
		b.handleUnchannel(s, i)
	case "ping":
		b.handlePing(s, i)
	}
}

// Add a summoner for tracking in a guild
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

	rankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error fetching summoner rank data: %v", err)
		respondWithError(s, i, fmt.Sprintf("Error fetching summoner rank data: %v", err))
		return
	}

	lastMatchData, err := b.riotClient.GetLastRankedSoloMatchData(account.SummonerPUUID)
	if err != nil {
		log.Printf("Error fetching last match data: %v", err)
	}

	if err := b.storage.AddSummoner(i.GuildID, i.ChannelID, summonerName, *summoner, rankInfo); err != nil {
		log.Printf("Error adding summoner to database: %v", err)
		respondWithError(s, i, fmt.Sprintf("Error adding summoner to database: %v", err))
		return
	}

	if lastMatchData != nil {
		if err := b.storage.AddMatch(summoner.RiotSummonerID, lastMatchData, rankInfo.LeaguePoints); err != nil {
			log.Printf("Error adding match to database: %v", err)
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Summoner '%s' (Level %d) is now being tracked in this server. Current Rank: %s %s %d LP",
				summonerName, summoner.SummonerLevel, rankInfo.Tier, rankInfo.Rank, rankInfo.LeaguePoints),
		},
	})
}

// Remove a summoner from the list of summoners tracked in the guild
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

// List all summoners from a guild with their respective ranks
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
		for _, summoner := range summoners {
			if summoner.Rank == "" || strings.ToUpper(summoner.Rank) == "UNRANKED" {
				content += fmt.Sprintf("- %s (Unranked)\n", summoner.Name)
			} else {
				words := strings.Fields(summoner.Rank)
				words[0] = utils.CapitalizeFirst(strings.ToLower(words[0]))
				formattedRank := strings.Join(words, " ")
				content += fmt.Sprintf("- %s (%s, %dLP)\n", summoner.Name, formattedRank, summoner.LeaguePoints)
			}
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

// Remove channel_id from guild table to let user select a new one
func (b *Bot) handleUnchannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := b.storage.RemoveChannelFromGuild(i.GuildID, i.ChannelID)
	if err != nil {
		log.Printf("Error removing channel association: %v", err)
		respondWithError(s, i, "An error occurred while removing the channel association. Please try again later.")
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "This channel won't be used for update anymore. Type `/add {summonername#tagline}` to set a new channel.",
		},
	})
}

// generate an ephemeral error message that is only shown to the user that typed a command
func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
