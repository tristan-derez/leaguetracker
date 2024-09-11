package bot

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	dg "github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
	s "github.com/tristan-derez/league-tracker/internal/storage"
	u "github.com/tristan-derez/league-tracker/internal/utils"
)

// TrackMatches continuously monitors and tracks matches for all summoners across all guilds.
// It runs indefinitely, periodically checking for new matches and announcing them to relevant guilds.
func (b *Bot) TrackMatches() {
	ticker := time.NewTicker(4 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			log.Println("Stopping match tracking")
			return
		case <-ticker.C:
			summoners, err := b.storage.GetAllSummonersWithGuilds()
			if err != nil {
				log.Printf("Error fetching summoners: %v", err)
				continue
			}

			if len(summoners) == 0 {
				log.Print("No summoner to track for now")
				continue
			}

			log.Printf("🕵️ Tracking matches for %d summoners", len(summoners))

			for _, summoner := range summoners {
				err := u.RetryWithBackoff(func() error {
					return b.checkSummonerUpdates(summoner)
				}, u.DefaultRetryConfig)
				if err != nil {
					if _, nonRetryable := err.(*u.NonRetryableError); nonRetryable {
						log.Printf("Non-retryable error occurred while checking matches for %s: %v", summoner.Summoner.Name, err)
					} else {
						log.Printf("Failed to check matches for %s after multiple retries: %v", summoner.Summoner.Name, err)
					}
				}
			}
		}
	}
}

func (b *Bot) checkSummonerUpdates(summoner s.SummonerWithGuilds) error {
	summonerUUID, err := b.storage.GetSummonerUUIDFromRiotID(summoner.Summoner.RiotSummonerID)
	if err != nil {
		return u.NewNonRetryableError(fmt.Errorf("error getting internal summonerUUID for %s: %w", summoner.Summoner.Name, err))
	}

	previousRank, err := b.storage.GetPreviousRank(summonerUUID)
	if err != nil {
		return u.NewNonRetryableError(fmt.Errorf("error getting previous rank: %w", err))
	}

	currentRankInfo, err := b.riotClient.GetSummonerRank(summoner.Summoner.RiotSummonerID)
	if err != nil {
		return fmt.Errorf("error fetching current rank for %s: %w", summoner.Summoner.Name, err)
	}

	matchFound, newMatch, err := b.checkForNewMatch(summoner.Summoner)
	if err != nil {
		if riotapi.IsRateLimitError(err) {
			return err // Retryable error
		}
		return u.NewNonRetryableError(err)
	}

	if matchFound {
		b.processNewMatch(summoner, newMatch, previousRank, currentRankInfo)
	} else if hasRankChanged(previousRank, currentRankInfo) {
		b.processRankChange(summoner, previousRank, currentRankInfo, summonerUUID)
	}

	return nil
}

func hasRankChanged(prev *s.PreviousRank, current *riotapi.LeagueEntry) bool {
	if prev == nil {
		return true
	}
	return prev.PrevTier != current.Tier ||
		prev.PrevRank != current.Rank ||
		prev.PrevLP != current.LeaguePoints
}

func (b *Bot) processRankChange(summoner s.SummonerWithGuilds, prev *s.PreviousRank, current *riotapi.LeagueEntry, summonerUUID uuid.UUID) {
	lpChange := b.storage.CalculateLPChange(prev.PrevTier, current.Tier, prev.PrevRank, current.Rank, prev.PrevLP, current.LeaguePoints)

	if err := b.storage.UpdateLeagueEntry(summonerUUID, current.LeaguePoints, current.Tier, current.Rank); err != nil {
		log.Printf("Error updating league entry for %s: %v", summoner.Summoner.Name, err)
	}

	if err := b.storage.CreateNewRowInLPHistory(summonerUUID, "DODGE", lpChange, current.LeaguePoints, current.Tier, current.Rank); err != nil {
		log.Printf("Error creating new row in lp history for %s: %v", summoner.Summoner.Name, err)
	}

	embed := b.prepareRankChangeEmbed(summoner.Summoner, prev, current, lpChange)

	for _, guildID := range summoner.GuildIDs {
		if err := b.announceNewMatch(guildID, embed); err != nil {
			log.Printf("Error announcing rank change for %s in guild %s: %v", summoner.Summoner.Name, guildID, err)
		}
	}

	log.Printf("%s probably dodged a game", summoner.Summoner.Name)
}

func (b *Bot) prepareRankChangeEmbed(summoner riotapi.Summoner, prev *s.PreviousRank, current *riotapi.LeagueEntry, lpChange int) *dg.MessageEmbed {
	winRate := u.CalculateWinRate(current.Wins, current.Losses)
	embedColor := 0xFF0000

	currentVersion, _ := b.riotClient.GetCurrentDDragonVersion()

	profileIconImageURL := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/img/profileicon/%d.png", currentVersion, summoner.ProfileIconID)

	oldRank := fmt.Sprintf("%s %s (%dlp)", prev.PrevTier, prev.PrevRank, prev.PrevLP)
	currentRank := fmt.Sprintf("%s %s (%dlp)", current.Tier, current.Rank, current.LeaguePoints)
	fullFooterStr := fmt.Sprintf("%s -> %s • %s", oldRank, currentRank, time.Now().Format("2006-01-02 15:04"))

	embed := &dg.MessageEmbed{
		Title:       fmt.Sprintf("%s (%+d LP)", summoner.Name, lpChange),
		Color:       embedColor,
		Description: "Probably a dodge 😱",
		Thumbnail: &dg.MessageEmbedThumbnail{
			URL: profileIconImageURL,
		},
		Fields: []*dg.MessageEmbedField{
			{
				Name:   "Wins",
				Value:  fmt.Sprintf("%d", current.Wins),
				Inline: true,
			},
			{
				Name:   "Losses",
				Value:  fmt.Sprintf("%d", current.Losses),
				Inline: true,
			},
			{
				Name:   "Win Rate",
				Value:  fmt.Sprintf("%.1f%%", winRate),
				Inline: true,
			},
		},
		Footer: &dg.MessageEmbedFooter{
			Text: fullFooterStr,
		},
	}

	return embed
}

// processNewMatch processes a new match
func (b *Bot) processNewMatch(summoner s.SummonerWithGuilds, newMatch *riotapi.MatchData, previousRank *s.PreviousRank, currentRankInfo *riotapi.LeagueEntry) {
	summonerUUID, err := b.storage.GetSummonerUUIDFromRiotID(summoner.Summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error getting internal summonerUUID for %s: %v", summoner.Summoner.Name, err)
		return
	}

	currentVersion, _ := b.riotClient.GetCurrentDDragonVersion()

	wasInPlacements := (previousRank == nil || (previousRank.PrevTier == "UNRANKED" && previousRank.PrevRank == ""))
	isRemake := newMatch.GameDuration < 210

	if wasInPlacements {
		if !isRemake {
			err = b.storage.IncrementPlacementGames(summonerUUID, newMatch.Win)
			if err != nil {
				log.Printf("Error incrementing placement games for %s: %v", summoner.Summoner.Name, err)
				return
			}
		}

		err = b.storage.AddPlacementMatch(summonerUUID, newMatch)
		if err != nil {
			log.Printf("Error adding placement match for %s: %v", summoner.Summoner.Name, err)
			return
		}

		updatedPlacementStatus, err := b.storage.GetCurrentPlacementGames(summonerUUID)
		if err != nil {
			log.Printf("Error getting updated placement status for %s: %v", summoner.Summoner.Name, err)
			return
		}

		var embed *dg.MessageEmbed
		if currentRankInfo.Tier != "UNRANKED" || updatedPlacementStatus.TotalGames == 5 {
			embed = b.preparePlacementCompletionEmbed(summoner.Summoner, newMatch, currentVersion, updatedPlacementStatus, currentRankInfo)

			err = b.storage.UpdateLeagueEntry(summonerUUID, currentRankInfo.LeaguePoints, currentRankInfo.Tier, currentRankInfo.Rank)
			if err != nil {
				log.Printf("Error updating summoner rank for %s: %v", summoner.Summoner.Name, err)
			}
		} else {
			embed = b.preparePlacementMatchEmbed(summoner.Summoner, newMatch, currentVersion, updatedPlacementStatus)
		}

		for _, guildID := range summoner.GuildIDs {
			if err := b.announceNewMatch(guildID, embed); err != nil {
				log.Printf("Error announcing new placement match for %s in guild %s: %v", summoner.Summoner.Name, guildID, err)
			}
		}

		return
	}

	lpChange, err := b.storage.AddMatchAndGetLPChange(summoner.Summoner.RiotSummonerID, newMatch, currentRankInfo.LeaguePoints, currentRankInfo.Rank, currentRankInfo.Tier)
	if err != nil {
		log.Printf("Error storing match and calculating LP change for %s: %v", summoner.Summoner.Name, err)
		return
	}

	embed := b.prepareMatchEmbed(summoner.Summoner, newMatch, currentRankInfo, lpChange, currentVersion, previousRank)

	for _, guildID := range summoner.GuildIDs {
		if err := b.announceNewMatch(guildID, embed); err != nil {
			log.Printf("Error announcing new match for %s in guild %s: %v", summoner.Summoner.Name, guildID, err)
		}
	}

	log.Printf("New match processed for %s in %d guilds", summoner.Summoner.Name, len(summoner.GuildIDs))
}

// checkForNewMatch checks if there's a new match for the given summoner.
func (b *Bot) checkForNewMatch(summoner riotapi.Summoner) (bool, *riotapi.MatchData, error) {
	storedMatchID, err := b.storage.GetLastMatchID(summoner.SummonerPUUID)
	if err != nil {
		return false, nil, fmt.Errorf("error getting stored match ID: %v", err)
	}

	hasNewMatch, newMatch, err := b.riotClient.GetNewMatchForSummoner(summoner.SummonerPUUID, storedMatchID)
	if err != nil {
		return false, nil, fmt.Errorf("error checking for new match: %v", err)
	}

	return hasNewMatch, newMatch, nil
}

// announceNewMatch sends the embed that was previously processed to the channel that was set for updates
func (b *Bot) announceNewMatch(guildID string, embed *dg.MessageEmbed) error {
	channelID, err := b.storage.GetGuildChannelID(guildID)
	if err != nil {
		return fmt.Errorf("error getting channel ID for guild %s: %w", guildID, err)
	}

	return u.RetryWithBackoff(func() error {
		_, err := b.session.ChannelMessageSendEmbed(channelID, embed)
		if err != nil {
			return fmt.Errorf("error sending embed message to channel %s: %w", channelID, err)
		}
		return nil
	}, u.DefaultRetryConfig)
}

// prepareMatchEmbed creates and returns a Discord message embed for a match.
// It takes summoner information, match data, rank info, LP change, current game version,
// and previous rank as input to generate a detailed embed about the match result.
func (b *Bot) prepareMatchEmbed(summoner riotapi.Summoner, match *riotapi.MatchData, rankInfo *riotapi.LeagueEntry, lpChange int, currentVersion string, previousRank *s.PreviousRank) *dg.MessageEmbed {
	winRate := u.CalculateWinRate(rankInfo.Wins, rankInfo.Losses)

	var lpChangeStr string
	if match.GameDuration < 210 {
		lpChangeStr = "Remake"
	} else {
		lpChangeStr = fmt.Sprintf("%+dLP", lpChange)
	}

	embedColor := getEmbedColor(match.Result, match.GameDuration)

	endOfGameStr := u.FormatTime(match.GameEndTimestamp)
	oldRank := fmt.Sprintf("%s %s (%dlp)", previousRank.PrevTier, previousRank.PrevRank, previousRank.PrevLP)
	currentRank := fmt.Sprintf("%s %s (%dlp)", rankInfo.Tier, rankInfo.Rank, rankInfo.LeaguePoints)
	fullFooterStr := fmt.Sprintf("%s -> %s • %s", oldRank, currentRank, endOfGameStr)
	TeamDmgOwnPercentage := fmt.Sprintf(" %.0f%% of team's damage", match.TeamDamagePercentage*100)
	leagueOfGraphURL := fmt.Sprintf("https://www.leagueofgraphs.com/match/euw/%s", strings.TrimPrefix(match.MatchID, "EUW1_"))
	displayChampionName := u.ChampionNameMapper(match.ChampionName, false)
	imageChampionName := u.ChampionNameMapper(match.ChampionName, true)
	championImageURL := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/img/champion/%s.png", currentVersion, imageChampionName)

	embed := &dg.MessageEmbed{
		Title:       fmt.Sprintf("**%s (%s)**", summoner.Name, lpChangeStr),
		URL:         leagueOfGraphURL,
		Description: fmt.Sprintf("**%d/%d/%d** with **%s** (%d:%02d) • %s and %.0f%%KP", match.Kills, match.Deaths, match.Assists, displayChampionName, match.GameDuration/60, match.GameDuration%60, TeamDmgOwnPercentage, match.KillParticipation*100),
		Color:       embedColor,
		Thumbnail: &dg.MessageEmbedThumbnail{
			URL: championImageURL,
		},
		Fields: []*dg.MessageEmbedField{
			{
				Name:   "Wins",
				Value:  fmt.Sprintf("%d", rankInfo.Wins),
				Inline: true,
			},
			{
				Name:   "Losses",
				Value:  fmt.Sprintf("%d", rankInfo.Losses),
				Inline: true,
			},
			{
				Name:   "Win Rate",
				Value:  fmt.Sprintf("%.1f%%", winRate),
				Inline: true,
			},
		},
		Footer: &dg.MessageEmbedFooter{
			Text: fullFooterStr,
		},
	}

	return embed
}

// preparePlacementMatchEmbed returns an embed for placement games
func (b *Bot) preparePlacementMatchEmbed(summoner riotapi.Summoner, match *riotapi.MatchData, currentVersion string, placementStatus *riotapi.PlacementStatus) *dg.MessageEmbed {
	kda := float64(match.Kills+match.Assists) / math.Max(float64(match.Deaths), 1)

	embedColor := getEmbedColor(match.Result, match.GameDuration)

	endOfGameStr := u.FormatTime(match.GameEndTimestamp)
	TeamDmgOwnPercentage := fmt.Sprintf(" %.0f%% of team's damage", match.TeamDamagePercentage*100)
	leagueOfGraphURL := fmt.Sprintf("https://www.leagueofgraphs.com/match/euw/%s", strings.TrimPrefix(match.MatchID, "EUW1_"))
	displayChampionName := u.ChampionNameMapper(match.ChampionName, false)
	imageChampionName := u.ChampionNameMapper(match.ChampionName, true)
	championImageURL := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/img/champion/%s.png", currentVersion, imageChampionName)

	var placementInfo string
	if match.GameDuration < 210 {
		placementInfo = "Remake"
	} else {
		placementInfo = fmt.Sprintf("Placement game %d/5 completed", placementStatus.TotalGames)
	}

	title := summoner.Name
	if placementInfo != "" {
		title = fmt.Sprintf("**%s • %s**", summoner.Name, placementInfo)
	}

	embed := &dg.MessageEmbed{
		Title:       title,
		URL:         leagueOfGraphURL,
		Description: fmt.Sprintf("**%d/%d/%d** (**%.2f:1** KDA) with **%s** (%d:%02d)", match.Kills, match.Deaths, match.Assists, kda, displayChampionName, match.GameDuration/60, match.GameDuration%60),
		Color:       embedColor,
		Thumbnail: &dg.MessageEmbedThumbnail{
			URL: championImageURL,
		},
		Fields: []*dg.MessageEmbedField{
			{
				Value:  fmt.Sprintf("**%d**CS (%dCS/min) • **%s** and **%.0f%%**KP", match.TotalMinionsKilled+match.NeutralMinionsKilled, (match.TotalMinionsKilled+match.NeutralMinionsKilled)/(match.GameDuration/60), TeamDmgOwnPercentage, match.KillParticipation*100),
				Inline: false,
			},
			{
				Value:  fmt.Sprintf("**%d** damage inflicted to champions", match.TotalDamageDealtToChampions),
				Inline: true,
			},
			{
				Value:  fmt.Sprintf("**%dW, %dL**", placementStatus.Wins, placementStatus.Losses),
				Inline: false,
			},
		},
		Footer: &dg.MessageEmbedFooter{
			Text: endOfGameStr,
		},
	}

	return embed
}

// preparePlacementCompletionEmbed returns an embed message for placement games completion
func (b *Bot) preparePlacementCompletionEmbed(summoner riotapi.Summoner, match *riotapi.MatchData, currentVersion string, placementStatus *riotapi.PlacementStatus, newRank *riotapi.LeagueEntry) *dg.MessageEmbed {
	championImageURL := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/img/champion/%s.png", currentVersion, match.ChampionName)
	embedColor := getEmbedColor(match.Result, match.GameDuration)
	leagueOfGraphURL := fmt.Sprintf("https://www.leagueofgraphs.com/match/euw/%s", strings.TrimPrefix(match.MatchID, "EUW1_"))
	kda := float64(match.Kills+match.Assists) / math.Max(float64(match.Deaths), 1)
	TeamDmgOwnPercentage := fmt.Sprintf("**%.0f%%** of team's damage", match.TeamDamagePercentage*100)

	endOfGameStr := u.LowercaseTime(match.GameEndTimestamp)

	embed := &dg.MessageEmbed{
		Title:       fmt.Sprintf("%s • Placement complete!", summoner.Name),
		URL:         leagueOfGraphURL,
		Description: fmt.Sprintf("**%d/%d/%d** (**%.2f:1** KDA) with **%s** (%d:%02d)", match.Kills, match.Deaths, match.Assists, kda, match.ChampionName, match.GameDuration/60, match.GameDuration%60),
		Color:       embedColor,
		Thumbnail: &dg.MessageEmbedThumbnail{
			URL: championImageURL,
		},
		Fields: []*dg.MessageEmbedField{
			{
				Value:  fmt.Sprintf("**%d**CS (%dCS/min) • %s and **%.0f%%**KP", match.TotalMinionsKilled+match.NeutralMinionsKilled, (match.TotalMinionsKilled+match.NeutralMinionsKilled)/(match.GameDuration/60), TeamDmgOwnPercentage, match.KillParticipation*100),
				Inline: false,
			},
			{
				Value:  fmt.Sprintf("From **UNRANKED** to **%s %s** (**%d** LP)", newRank.Tier, newRank.Rank, newRank.LeaguePoints),
				Inline: false,
			},
			{
				Value:  fmt.Sprintf("**%dW/%dL**", placementStatus.Wins, placementStatus.Losses),
				Inline: false,
			},
		},
		Footer: &dg.MessageEmbedFooter{
			Text: fmt.Sprintf("Placements completed %s", endOfGameStr),
		},
	}

	return embed
}

// getEmbedColor returns an appropriate color code (in hexadecimal format) based on the match result and game duration.
//   - If the game duration is less than 210 seconds, it returns grey (0x808080), indicating the match was a remake. (https://leagueoflegends.fandom.com/wiki/Surrendering)
//   - If the match result is "win" (case-insensitive), it returns green (0x00FF00), indicating a win.
//   - If the match result is "loss" (case-insensitive), it returns red (0xFF0000), indicating a loss.
//   - For any other cases (e.g., unknown match result), it returns grey (0x808080).
func getEmbedColor(matchResult string, gameDuration int) int {
	switch {
	case gameDuration < 210:
		return 0x808080
	case strings.ToLower(matchResult) == "win":
		return 0x00FF00
	case strings.ToLower(matchResult) == "loss":
		return 0xFF0000
	default:
		return 0x808080
	}
}
