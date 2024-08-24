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

// TrackMatches continuously monitors and tracks matches for all summoners in a given guild.
// It runs indefinitely, periodically checking for new summoners and initiating tracking for them.
func (b *Bot) TrackMatches(guildID string) error {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	trackedSummoners := make(map[string]bool)

	for range ticker.C {
		summoners, err := b.storage.GetAllSummonersForGuild(guildID)
		if err != nil {
			log.Printf("Error getting summoners: %v", err)
			continue
		}

		log.Printf("🕵️ Tracking summoners in guild id: %v", guildID)

		for _, summoner := range summoners {
			if !trackedSummoners[summoner.SummonerPUUID] {
				trackedSummoners[summoner.SummonerPUUID] = true
				go func(s riotapi.Summoner) {
					for {
						matchFound, err := b.trackSummonerMatches(guildID, s)
						if err != nil {
							log.Printf("Error tracking matches for summoner %s: %v", s.SummonerPUUID, err)
							time.Sleep(30 * time.Second)
							continue
						}

						if matchFound {
							time.Sleep(15 * time.Minute)
						} else {
							time.Sleep(2 * time.Minute)
						}
					}
				}(summoner)
			}
		}
	}

	return nil
}

// trackSummonerMatches tracks matches for a summoner, using exponential backoff
// for rate limit errors. It retries up to 5 times, with delays ranging from 1 second to 15 minutes.
// Returns true if a match was found and tracked, false otherwise. Non-rate-limit errors
// are returned immediately without retrying.
func (b *Bot) trackSummonerMatches(guildID string, summoner riotapi.Summoner) (bool, error) {
	backoffStrategy := backoff.NewExponentialBackOff()
	backoffStrategy.InitialInterval = 500 * time.Millisecond
	backoffStrategy.MaxInterval = 5 * time.Second
	backoffStrategy.MaxElapsedTime = 30 * time.Second

	var matchFound bool
	var lastErr error

	operation := func() error {
		var err error
		matchFound, err = b.performSummonerMatchCheck(guildID, summoner)
		if err != nil {
			if riotapi.IsRateLimitError(err) {
				return err // Retryable error
			}
			lastErr = err
			return backoff.Permanent(err) // Non-retryable error
		}
		return nil // Success
	}

	err := backoff.Retry(operation, backoffStrategy)
	if err != nil {
		if lastErr != nil {
			return false, fmt.Errorf("failed to track matches for %s: %v", summoner.Name, lastErr)
		}
		return false, fmt.Errorf("failed to track matches for %s after multiple retries: %v", summoner.Name, err)
	}

	return matchFound, nil
}

// performSummonerMatchCheck checks for new matches for a given summoner and processes them.
// It fetches the last known match, checks for new ones, updates the database,
// and announces the new match in the guild's channel.
func (b *Bot) performSummonerMatchCheck(guildID string, summoner riotapi.Summoner) (bool, error) {
	storedMatchID, err := b.storage.GetLastMatchID(summoner.SummonerPUUID)
	if err != nil {
		log.Printf("Error getting stored match ID for %s: %v", summoner.Name, err)
		return false, err
	}

	hasNewMatch, newMatch, err := b.riotClient.GetNewMatchForSummoner(summoner.SummonerPUUID, storedMatchID)
	if err != nil {
		return false, fmt.Errorf("error checking for new match for %s: %v", summoner.Name, err)
	}

	if !hasNewMatch {
		return false, nil
	}

	summonerID, err := b.storage.GetSummonerIDFromRiotID(summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error getting internal summonerID for %s: %v", summoner.Name, err)
		return false, err
	}

	// Get previous rank before adding the new match
	previousRank, err := b.storage.GetPreviousRank(summonerID)
	if err != nil {
		log.Printf("Error getting previous rank: %v", err)
		return false, err
	}

	currentRankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error fetching current rank for %s: %v", summoner.Name, err)
		return false, err
	}

	// Add the new match to the database and get the LP change
	lpChange, err := b.storage.AddMatchAndGetLPChange(summoner.RiotSummonerID, newMatch, currentRankInfo.LeaguePoints, currentRankInfo.Rank, currentRankInfo.Tier)
	if err != nil {
		log.Printf("Error storing match and calculating LP change for %s: %v", summoner.Name, err)
		return false, err
	}

	// Get current version for champion image
	currentVersion, err := b.riotClient.GetCurrentDDragonVersion()
	if err != nil {
		log.Printf("Warning: %v\n", err)
	}

	// Prepare the embed for the announcement
	embed := b.prepareMatchEmbed(summoner, newMatch, currentRankInfo, lpChange, currentVersion, previousRank)

	// Announce the new match
	if err := b.announceNewMatch(guildID, embed); err != nil {
		log.Printf("Error announcing new match for %s: %v", summoner.Name, err)
	}

	log.Printf("New match processed for %s in guild id: %v", summoner.Name, guildID)
	return true, nil
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
