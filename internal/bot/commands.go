package bot

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
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

	if err := respondToInteractionWithSource(s, i, "Adding summoner(s)..."); err != nil {
		log.Printf("Error responding to interaction: %v", err)
		respondWithError(s, i, "Something went wrong. Please try again later.")
		return
	}

	resultChan := make(chan string, 1)

	go func() {
		var responses []string

		for _, summonerName := range summonerNames {
			response := b.processSingleSummoner(summonerName, i.GuildID, i.ChannelID)
			responses = append(responses, response)
		}

		fullResponse := strings.Join(responses, "\n\n")
		resultChan <- fullResponse
	}()

	select {
	case result := <-resultChan:
		// handle 2000 chars limit from discord
		chunks := utils.ChunkMessage(result, 2000)
		for _, chunk := range chunks {
			if err := sendFollowUpMessage(s, i, chunk); err != nil {
				log.Printf("Error sending follow-up message: %v", err)
			}
		}
	case <-time.After(40 * time.Second):
		message := "The operation is taking longer than expected. The summoners are being processed in the background. Please check the tracked summoners list later."
		if err := sendFollowUpMessage(s, i, message); err != nil {
			log.Printf("Error sending follow-up message: %v", err)
		}
	}
}

func (b *Bot) processSingleSummoner(summonerName, guildID, channelID string) string {
	summonerName = strings.TrimSpace(summonerName)
	parts := strings.SplitN(summonerName, "#", 2)

	if len(parts) != 2 {
		return fmt.Sprintf("❌ Invalid format for '%s'. Use Name#Tag.", summonerName)
	}

	gameName := strings.TrimSpace(parts[0])
	tagLine := strings.TrimSpace(parts[1])

	// Check if the summoner exists and associate them with the guild if they do
	summonerUUID, exists, err := b.storage.GetSummonerUUIDAndAssociate(guildID, channelID, summonerName)
	if err != nil {
		log.Printf("Error checking or associating summoner: %v", err)
		return "❌ Error processing summoner."
	}

	if exists {
		rankInfo, err := b.storage.GetLeagueEntry(summonerUUID)
		if err != nil {
			log.Printf("Error fetching league entry for '%s': %v", summonerName, err)
			return "❌ Error fetching summoner rank."
		}

		return b.formatSummonerResponse(summonerName, rankInfo, uuid.Nil, "")
	}

	account, err := b.riotClient.GetAccountPUUIDBySummonerName(gameName, tagLine)
	if err != nil {
		return fmt.Sprintf("❌ Unable to find '%s': %v", summonerName, err)
	}

	fullNameOriginalCasing := fmt.Sprintf("%s#%s", account.SummonerName, account.SummonerTagLine)

	summoner, err := b.riotClient.GetSummonerByPUUID(account.SummonerPUUID)
	if err != nil {
		log.Printf("Error fetching details for '%s': %v", summonerName, err)
	}

	rankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error fetching rank for '%s': %v", summonerName, err)
	}

	summonerUUID, err = b.storage.AddSummoner(guildID, channelID, fullNameOriginalCasing, *summoner, rankInfo)
	if err != nil {
		log.Printf("Error adding '%s' to database: %v", summonerName, err)
		return fmt.Sprintf("❌ Error adding '%s' to database.", summonerName)
	}

	go b.addLastMatchData(summoner.RiotSummonerID, account.SummonerPUUID, *rankInfo)

	return b.formatSummonerResponse(fullNameOriginalCasing, rankInfo, summonerUUID, account.SummonerPUUID)
}

func (b *Bot) formatSummonerResponse(summonerName string, rankInfo *riotapi.LeagueEntry, summonerUUID uuid.UUID, summonerPUUID string) string {
	if rankInfo.Tier == "UNRANKED" && rankInfo.Rank == "" {
		placementStatus, err := b.riotClient.GetPlacementStatus(summonerPUUID)
		if err != nil {
			log.Printf("Error fetching placement status for '%s': %v", summonerName, err)
			return fmt.Sprintf("❌ Error fetching placement status for %s", summonerName)
		}

		currentSeason := b.storage.GetCurrentSeason()
		err = b.storage.InitializePlacementGames(summonerUUID, currentSeason, placementStatus)
		if err != nil {
			log.Printf("Error initializing placement games for summoner %s: %v", summonerName, err)
			return fmt.Sprintf("❌ Error while initializing placement games for %s", summonerName)
		}

		if placementStatus.IsInPlacements {
			return fmt.Sprintf("✅ '%s' added. Currently in placement games (%d/5 completed)",
				summonerName, placementStatus.TotalGames)
		}
	}

	return fmt.Sprintf("✅ '%s' added. %s %s %d LP",
		summonerName, rankInfo.Tier, rankInfo.Rank, rankInfo.LeaguePoints)
}

func (b *Bot) addLastMatchData(summonerID, puuid string, rankInfo riotapi.LeagueEntry) {
	lastMatchData, err := b.riotClient.GetLastRankedSoloMatchData(puuid)
	if err != nil {
		log.Printf("error retrieving ranked games for '%s': %v", summonerID, err)
		return
	}

	if lastMatchData != nil {
		_, err := b.storage.AddMatchAndGetLPChange(summonerID, lastMatchData, rankInfo.LeaguePoints, rankInfo.Rank, rankInfo.Tier)
		if err != nil {
			log.Printf("Error adding match data for '%s': %v", summonerID, err)
		}
	}
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

	if err := respondToInteractionWithSource(s, i, "Removing summoner(s)..."); err != nil {
		log.Printf("Error responding to interaction: %v", err)
		respondWithError(s, i, "Something went wrong. Please try again later.")
		return
	}

	go func() {
		var responses []string
		guildID := i.GuildID

		for _, summonerName := range summonerNames {
			summonerName = strings.TrimSpace(summonerName)

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

		finalResponse := strings.Join(responses, "\n")

		if err := sendFollowUpMessage(s, i, finalResponse); err != nil {
			log.Printf("Error sending follow-up message: %v", err)
		}
	}()
}

// handleReset processes the /reset command for the Discord bot.
// It removes every summoners in guild from the bot's tracking system.
func (b *Bot) handleReset(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID := i.GuildID

	if err := respondToInteractionWithSource(s, i, "Removing every summoners from this guild..."); err != nil {
		log.Printf("Error responding to interaction: %v", err)
		respondWithError(s, i, "Something went wrong. Please try again later.")
		return
	}

	err := b.storage.RemoveAllSummoners(guildID)
	if err != nil {
		log.Printf("Error resetting summoners for guild %s: %v", guildID, err)
		respondWithError(s, i, "Something went wrong. Please try again later.")
		return
	}

	sendFollowUpMessage(s, i, "All summoners have been removed from tracking in this server.")
}

// handleList processes the /list command for the Discord bot.
// It display every summoners followed in the server.
func (b *Bot) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	guildID := i.GuildID

	if err := respondToInteractionWithSource(s, i, "Retrieving list of summoners in this guild..."); err != nil {
		return
	}

	go func() {
		summoners, err := b.storage.ListSummoners(guildID)
		if err != nil {
			log.Printf("Error listing summoners: %v", err)
			sendFollowUpMessage(s, i, "An error occurred while retrieving the list of summoners. Please try again later.")
			return
		}

		if len(summoners) == 0 {
			sendFollowUpMessage(s, i, "No summoners are being tracked in this server.")
			return
		}

		currentVersion, err := b.riotClient.GetCurrentDDragonVersion()
		if err != nil {
			log.Printf("Warning: %v", err)
		}

		embeds := []*discordgo.MessageEmbed{}

		for _, summoner := range summoners {
			var description string
			var title string

			profileIconImageURL := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/img/profileicon/%d.png", currentVersion, summoner.ProfileIconID)

			urlFormattedName := strings.ReplaceAll(summoner.Name, "#", "-")
			leagueOfGraphLink := fmt.Sprintf("https://www.leagueofgraphs.com/summoner/euw/%s", url.PathEscape(urlFormattedName))

			color := utils.GetRankColor(summoner.Rank)

			if summoner.Rank == "" || strings.ToUpper(summoner.Rank) == "UNRANKED" {
				title = summoner.Name
				description = "Unranked"
			} else {
				words := strings.Fields(summoner.Rank)
				words[0] = utils.CapitalizeFirst(strings.ToLower(words[0]))
				formattedRank := strings.Join(words, " ")
				title = summoner.Name
				description = fmt.Sprintf("%s, %d LP", formattedRank, summoner.LeaguePoints)
			}

			embed := &discordgo.MessageEmbed{
				Title:       title,
				URL:         leagueOfGraphLink,
				Color:       color,
				Description: description,
				Thumbnail: &discordgo.MessageEmbedThumbnail{
					URL: profileIconImageURL,
				},
			}

			embeds = append(embeds, embed)
		}

		// Split embeds into chunks of 10
		for idx := 0; idx < len(embeds); idx += 10 {
			end := idx + 10
			if end > len(embeds) {
				end = len(embeds)
			}
			chunkEmbeds := embeds[idx:end]

			if err := sendFollowUpMessage(s, i, "", chunkEmbeds...); err != nil {
				log.Printf("Error sending follow-up message: %v", err)
			}
		}
	}()
}

// handleUnchannel processes the /unchannel command for the Discord bot.
// It removes the associated channel from the guild
// (where the bot display new matches from summoners).
func (b *Bot) handleUnchannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := b.storage.RemoveChannelFromGuild(i.GuildID, i.ChannelID)
	if err != nil {
		log.Printf("Error removing channel association: %v", err)
		respondWithError(s, i, "Something went wrong. Please try again later.")
		return
	}

	if err := respondToInteractionWithSource(s, i, "This channel won't be used for update anymore. Type `/add {summonername#tagline}` to set a new channel."); err != nil {
		log.Printf("Error responding to interaction: %v", err)
		respondWithError(s, i, "Something went wrong. Please try again later.")
		return
	}
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

// sendFollowUpMessage sends a follow-up message to a Discord interaction
func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string, embeds ...*discordgo.MessageEmbed) error {
	params := &discordgo.WebhookParams{
		Content: content,
		Embeds:  embeds,
	}

	_, err := s.FollowupMessageCreate(i.Interaction, true, params)
	if err != nil {
		log.Printf("Error sending follow-up message: %v", err)
		return err
	}
	return nil
}

func respondToInteractionWithSource(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
	if err != nil {
		log.Printf("Error acknowledging interaction: %v", err)
		return err
	}
	return nil
}
