package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
	"github.com/tristan-derez/league-tracker/internal/storage"
	"github.com/tristan-derez/league-tracker/internal/utils"
)

// handleInteraction is a method of the Bot struct that handles Discord interactions
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
	}
}

// handleAdd processes the "add" command for the Discord bot.
// It adds one or more summoners to the bot's tracking system.
func (b *Bot) handleAdd(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondWithError(s, i, "Please provide at least one summoner name.")
		return
	}

	summonerNames := strings.Split(options[0].StringValue(), ",")

	// Respond immediately to avoid Discord interaction timeout
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Adding summoner(s)...",
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
		return
	}

	// Process summoners asynchronously
	go func() {
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
				if riotapi.IsRateLimitError(err) {
					log.Printf("Rate limit exceeded. Please try again later.")
					break
				}
				if apiErr, ok := err.(*riotapi.RiotAPIError); ok {
					log.Printf(apiErr.Message)
				} else {
					responses = append(responses, fmt.Sprintf("❌ Unable to find '%s': %v", summonerName, err))
				}
				continue
			}

			summoner, err := b.riotClient.GetSummonerByPUUID(account.SummonerPUUID)
			if err != nil {
				log.Printf("Error fetching details for '%s': %v", summonerName, err)
				continue
			}

			rankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
			if err != nil {
				log.Printf("Error fetching rank for '%s': %v", summonerName, err)
				continue
			}

			if err := b.storage.AddSummoner(i.GuildID, i.ChannelID, summonerName, *summoner, rankInfo); err != nil {
				responses = append(responses, fmt.Sprintf("❌ Error adding '%s' to database.", summonerName))
				log.Printf("Error adding summoner '%s' to database: %v", summonerName, err)
				continue
			}

			if rankInfo.Tier == "UNRANKED" && rankInfo.Rank == "" {
				placementStatus, err := b.riotClient.GetPlacementStatus(account.SummonerPUUID)
				if err != nil {
					log.Printf("Error fetching placement status for '%s': %v", summonerName, err)
					continue
				}

				currentSeason := b.storage.GetCurrentSeason()
				summonerUUID, err := b.storage.GetSummonerUUIDFromRiotID(summoner.RiotSummonerID)
				if err != nil {
					log.Printf("Error getting summoner uuid for '%s': %v", summonerName, err)
					continue
				}

				err = b.storage.InitializePlacementGames(summonerUUID, currentSeason, placementStatus)
				if err != nil {
					log.Printf("Error initializing placement games for summoner %s: %v", summonerUUID, err)
					continue
				}

				if placementStatus.IsInPlacements {
					responses = append(responses, fmt.Sprintf("✅ '%s' added. Currently in placement games (%d/5 completed)",
						summonerName, placementStatus.TotalGames))
				}
			} else {
				responses = append(responses, fmt.Sprintf("✅ '%s' added. %s %s %d LP",
					summonerName, rankInfo.Tier, rankInfo.Rank, rankInfo.LeaguePoints))
			}

			lastMatchData, err := b.riotClient.GetLastRankedSoloMatchData(account.SummonerPUUID)
			if err != nil {
				log.Printf("error retrieving ranked games for '%s': %v", summonerName, err)
			}

			if lastMatchData != nil {
				_, err := b.storage.AddMatchAndGetLPChange(summoner.RiotSummonerID, lastMatchData, rankInfo.LeaguePoints, rankInfo.Rank, rankInfo.Tier)
				if err != nil {
					log.Printf("Error adding match data for '%s': %v", summonerName, err)
				}
			} else {
				log.Printf("No recent ranked matches found for summoner %s", summonerName)
			}
		}

		var messageChunks []string
		currentChunk := ""
		for _, response := range responses {
			if len(currentChunk)+len(response)+2 > 2000 { // +2 for "\n\n"
				messageChunks = append(messageChunks, currentChunk)
				currentChunk = response
			} else {
				if currentChunk != "" {
					currentChunk += "\n\n"
				}
				currentChunk += response
			}
		}
		if currentChunk != "" {
			messageChunks = append(messageChunks, currentChunk)
		}

		for _, chunk := range messageChunks {
			_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: chunk,
			})
			if err != nil {
				log.Printf("Error sending follow-up message: %v", err)
			}
		}
	}()
}

// handleRemove processes the /remove command for the Discord bot.
// It removes one or more summoners from the bot's tracking system.
func (b *Bot) handleRemove(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondWithError(s, i, "Please provide at least one summoner name.")
		return
	}

	summonerNames := strings.Split(options[0].StringValue(), ",")

	// Respond immediately to avoid timeout
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Removing summoner(s)...",
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
		return
	}

	// Process summoners in a separate goroutine
	go func() {
		var responses []string
		guildID := i.GuildID

		for _, summonerName := range summonerNames {
			summonerName = strings.TrimSpace(strings.ToLower(summonerName))

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

		// Send the final response as a follow-up message
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: finalResponse,
		})
		if err != nil {
			log.Printf("Error sending follow-up message: %v", err)
		}
	}()
}

// handleReset processes the /reset command for the Discord bot.
// It removes every summoners from the bot's tracking system.
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

// sendFollowUpMessage sends a follow-up message to a Discord interaction
func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
	})
	if err != nil {
		log.Printf("Error sending follow-up message: %v", err)
	}
}

// handleList processes the /list command for the Discord bot.
// It display every summoners followed in the server.
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
		// Format the list of summoners with their ranks and LP
		for _, summoner := range summoners {
			if summoner.Rank == "" || strings.ToUpper(summoner.Rank) == "UNRANKED" {
				content += fmt.Sprintf("- %s (Unranked)\n", summoner.Name)
			} else {
				// Format the tier, rank and lp string
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

// handleUnchannel processes the /unchannel command for the Discord bot.
// It removes the associated channel from the guild
// (where the bot display new matches from summoners).
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

// respondWithError generates an ephemeral error message that is only shown to the user that typed a command
func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
