package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/tristan-derez/league-tracker/internal/storage"
	"github.com/tristan-derez/league-tracker/internal/utils"
)

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "add":
		b.handleAdd(s, i)
	case "remove":
		b.handleRemove(s, i)
	case "reset":
		b.handleReset(s, i)
	case "list":
		b.handleList(s, i)
	case "unchannel":
		b.handleUnchannel(s, i)
	case "ping":
		b.handlePing(s, i)
	}
}

// add one or more users
func (b *Bot) handleAdd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondWithError(s, i, "Please provide at least one summoner name.")
		return
	}

	summonerNames := strings.Split(options[0].StringValue(), ",")
	var responses []string

	for _, summonerName := range summonerNames {
		summonerName = strings.TrimSpace(strings.ToLower(summonerName))
		parts := strings.SplitN(summonerName, "#", 2)

		if len(parts) != 2 {
			responses = append(responses, fmt.Sprintf("❌ Invalid format for '%s'. Use Name#Tag.", summonerName))
			continue
		}

		gameName := strings.TrimSpace(parts[0])
		tagLine := strings.TrimSpace(parts[1])

		account, err := b.riotClient.GetAccountPUUIDBySummonerName(gameName, tagLine)
		if err != nil {
			if strings.Contains(err.Error(), "rate limit exceeded") {
				responses = append(responses, "⚠️ Rate limit exceeded. Please try again later.")
				break // Stop processing further summoners
			}
			responses = append(responses, fmt.Sprintf("❌ Unable to find '%s'.", summonerName))
			continue
		}

		summoner, err := b.riotClient.GetSummonerByPUUID(account.SummonerPUUID)
		if err != nil {
			responses = append(responses, fmt.Sprintf("❌ Error fetching details for '%s'.", summonerName))
			continue
		}

		rankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
		if err != nil {
			responses = append(responses, fmt.Sprintf("❌ Error fetching rank for '%s'.", summonerName))
			continue
		}

		if err := b.storage.AddSummoner(i.GuildID, i.ChannelID, summonerName, *summoner, rankInfo); err != nil {
			responses = append(responses, fmt.Sprintf("❌ Error adding '%s' to database.", summonerName))
			continue
		}

		responses = append(responses, fmt.Sprintf("✅ '%s' (Level %d) added. Rank: %s %s %d LP",
			summonerName, summoner.SummonerLevel, rankInfo.Tier, rankInfo.Rank, rankInfo.LeaguePoints))

		lastMatchData, _ := b.riotClient.GetLastRankedSoloMatchData(account.SummonerPUUID)
		if lastMatchData != nil {
			b.storage.AddMatchAndGetLPChange(summoner.RiotSummonerID, lastMatchData, rankInfo.LeaguePoints, rankInfo.Rank, rankInfo.Tier)
		}
	}

	finalResponse := strings.Join(responses, "\n")

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: finalResponse,
		},
	})
}

// Remove one or more summoner(s) from the list of summoners tracked in the guild
func (b *Bot) handleRemove(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondWithError(s, i, "Please provide at least one summoner name.")
		return
	}

	summonerNames := strings.Split(options[0].StringValue(), ",")
	var responses []string

	for _, summonerName := range summonerNames {
		summonerName = strings.TrimSpace(strings.ToLower(summonerName))
		guildID := i.GuildID

		err := b.storage.RemoveSummoner(guildID, summonerName)
		if err != nil {
			if err == storage.ErrSummonerNotFound {
				responses = append(responses, fmt.Sprintf("❌ Summoner '%s' was not found in the tracking list.", summonerName))
			} else {
				log.Printf("Error removing summoner '%s': %v", summonerName, err)
				responses = append(responses, fmt.Sprintf("❌ An error occurred while removing '%s'. Please try again later.", summonerName))
			}
		} else {
			responses = append(responses, fmt.Sprintf("✅ Summoner '%s' has been removed from tracking in this server.", summonerName))
		}
	}

	// Join all responses into a single message
	finalResponse := strings.Join(responses, "\n")

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: finalResponse,
		},
	})
}

// handleReset removes all summoners followed within the guild
func (b *Bot) handleReset(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Respond immediately to avoid timeout
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Processing reset command...",
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
		return
	}

	guildID := i.GuildID

	// Remove all summoners
	err = b.storage.RemoveAllSummoners(guildID)
	if err != nil {
		log.Printf("Error resetting summoners for guild %s: %v", guildID, err)
		sendFollowUpMessage(s, i, "An error occurred while resetting summoners. Please try again later.")
		return
	}

	// Send a success message
	sendFollowUpMessage(s, i, "All summoners have been removed from tracking in this server.")
}

func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
	})
	if err != nil {
		log.Printf("Error sending follow-up message: %v", err)
	}
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
