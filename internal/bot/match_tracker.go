package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	dg "github.com/bwmarrin/discordgo"
	"github.com/cenkalti/backoff/v4"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
	s "github.com/tristan-derez/league-tracker/internal/storage"
	u "github.com/tristan-derez/league-tracker/internal/utils"
)

// TrackMatches continuously monitors and tracks matches for all summoners across all guilds.
// It runs indefinitely, periodically checking for new matches and announcing them to relevant guilds.
func (b *Bot) TrackMatches() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		summoners, err := b.storage.GetAllSummonersWithGuilds()
		if err != nil {
			log.Printf("Error getting summoners: %v", err)
			continue
		}

		log.Printf("🕵️ Tracking matches for %d summoners", len(summoners))

		for _, summoner := range summoners {
			go func(s s.SummonerWithGuilds) {
				backoffStrategy := backoff.NewExponentialBackOff()
				backoffStrategy.InitialInterval = 500 * time.Millisecond
				backoffStrategy.MaxInterval = 5 * time.Second
				backoffStrategy.MaxElapsedTime = 30 * time.Second

				operation := func() error {
					matchFound, newMatch, err := b.checkForNewMatch(s.Summoner)
					if err != nil {
						if riotapi.IsRateLimitError(err) {
							return err // Retryable error
						}
						return backoff.Permanent(err) // Non-retryable error
					}

					if matchFound {
						b.processAndAnnounceNewMatch(s, newMatch)
					}

					return nil // Success
				}

				err := backoff.Retry(operation, backoffStrategy)
				if err != nil {
					log.Printf("Failed to track matches for %s after multiple retries: %v", s.Summoner.Name, err)
				}
			}(summoner)
		}
	}
}

// processAndAnnounceNewMatch processes a new match and announces it to all relevant guilds.
func (b *Bot) processAndAnnounceNewMatch(summoner s.SummonerWithGuilds, newMatch *riotapi.MatchData) {
	summonerID, err := b.storage.GetSummonerIDFromRiotID(summoner.Summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error getting internal summonerID for %s: %v", summoner.Summoner.Name, err)
		return
	}

	previousRank, err := b.storage.GetPreviousRank(summonerID)
	if err != nil {
		log.Printf("Error getting previous rank: %v", err)
		return
	}

	currentRankInfo, err := b.riotClient.GetSummonerRank(summoner.Summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error fetching current rank for %s: %v", summoner.Summoner.Name, err)
		return
	}

	lpChange, err := b.storage.AddMatchAndGetLPChange(summoner.Summoner.RiotSummonerID, newMatch, currentRankInfo.LeaguePoints, currentRankInfo.Rank, currentRankInfo.Tier)
	if err != nil {
		log.Printf("Error storing match and calculating LP change for %s: %v", summoner.Summoner.Name, err)
		return
	}

	currentVersion, err := b.riotClient.GetCurrentDDragonVersion()
	if err != nil {
		log.Printf("Warning: %v", err)
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

// announceNewMatch sends the embed to the channel that was set for updates
func (b *Bot) announceNewMatch(guildID string, embed *dg.MessageEmbed) error {
	channelID, err := b.storage.GetGuildChannelID(guildID)
	if err != nil {
		return fmt.Errorf("error getting channel ID for guild %s: %w", guildID, err)
	}

	_, err = b.session.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return fmt.Errorf("error sending embed message to channel %s: %w", channelID, err)
	}

	return nil
}

// prepareMatchEmbed creates and returns a Discord message embed for a match.
// It takes summoner information, match data, rank info, LP change, current game version,
// and previous rank as input to generate a detailed embed about the match result.
func (b *Bot) prepareMatchEmbed(summoner riotapi.Summoner, match *riotapi.MatchData, rankInfo *riotapi.LeagueEntry, lpChange int, currentVersion string, previousRank *s.PreviousRank) *dg.MessageEmbed {
	// Calculate win rate
	totalGames := rankInfo.Wins + rankInfo.Losses
	var winRate float64
	if totalGames > 0 {
		winRate = float64(rankInfo.Wins) / float64(totalGames) * 100
	}

	championImageURL := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/img/champion/%s.png", currentVersion, match.ChampionName)

	// Determine embed color based on match result
	var embedColor int
	switch {
	case match.GameDuration < 240:
		embedColor = 0x808080 // Grey
	case strings.ToLower(match.Result) == "win":
		embedColor = 0x00FF00 // Green
	case strings.ToLower(match.Result) == "loss":
		embedColor = 0xFF0000 // Red
	default:
		embedColor = 0x808080 // Grey
	}

	lpChangeStr := fmt.Sprintf("%+d", lpChange)
	endOfGameStr := u.FormatTime(match.GameEndTimestamp)
	oldRank := fmt.Sprintf("%s %s (%dlp)", previousRank.PrevTier, previousRank.PrevRank, previousRank.PrevLP)
	currentRank := fmt.Sprintf("%s %s (%dlp)", rankInfo.Tier, rankInfo.Rank, rankInfo.LeaguePoints)
	fullFooterStr := fmt.Sprintf("%s -> %s • %s", oldRank, currentRank, endOfGameStr)
	TeamDmgOwnPercentage := fmt.Sprintf(" %.0f%% of team's damage", match.TeamDamagePercentage*100)
	leagueOfGraphLink := fmt.Sprintf("https://www.leagueofgraphs.com/match/euw/%s", strings.TrimPrefix(match.MatchID, "EUW1_"))

	// Create the embed
	embed := &dg.MessageEmbed{
		Description: fmt.Sprintf("**[%s (%s LP)](%s)**", summoner.Name, lpChangeStr, leagueOfGraphLink),
		Color:       embedColor,
		Thumbnail: &dg.MessageEmbedThumbnail{
			URL: championImageURL,
		},
		Fields: []*dg.MessageEmbedField{
			{
				Value:  fmt.Sprintf("**%d/%d/%d** with **%s** (%d:%02d) • %s and %.0f%%KP", match.Kills, match.Deaths, match.Assists, match.ChampionName, match.GameDuration/60, match.GameDuration%60, TeamDmgOwnPercentage, match.KillParticipation*100),
				Inline: false,
			},
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
