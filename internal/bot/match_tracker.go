package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	dg "github.com/bwmarrin/discordgo"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
	s "github.com/tristan-derez/league-tracker/internal/storage"
	u "github.com/tristan-derez/league-tracker/internal/utils"
)

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

		log.Printf("Tracking summoners...")

		for _, summoner := range summoners {
			if !trackedSummoners[summoner.SummonerPUUID] {
				trackedSummoners[summoner.SummonerPUUID] = true
				go func(s riotapi.Summoner) {
					for {
						b.trackSummonerMatches(guildID, s)
						time.Sleep(2 * time.Minute)
					}
				}(summoner)
			}
		}
	}

	return nil
}

func (b *Bot) trackSummonerMatches(guildID string, summoner riotapi.Summoner) {
	log.Printf("Checking for new ranked solo/duo matches for %s", summoner.Name)

	storedMatchID, err := b.storage.GetLastMatchID(summoner.SummonerPUUID)
	if err != nil {
		log.Printf("Error getting stored match ID for %s: %v", summoner.Name, err)
		return
	}

	hasNewMatch, newMatch, err := b.riotClient.GetNewMatchForSummoner(summoner.SummonerPUUID, storedMatchID)
	if err != nil {
		if strings.Contains(err.Error(), "rate limit exceeded") {
			log.Printf("Rate limit exceeded for %s, retrying in 2 minutes", summoner.Name)
		} else {
			log.Printf("Error checking for new match for %s: %v", summoner.Name, err)
		}
		return
	}

	if !hasNewMatch {
		return
	}

	summonerID, err := b.storage.GetSummonerIDFromRiotID(summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error getting internal summonerID for %s: %v", summoner.Name, err)
		return
	}

	previousRank, err := b.storage.GetPreviousRank(summonerID)
	if err != nil {
		log.Printf("Error getting previous rank: %v", err)
	}

	currentRankInfo, err := b.riotClient.GetSummonerRank(summoner.RiotSummonerID)
	if err != nil {
		log.Printf("Error fetching current rank for %s: %v", summoner.Name, err)
		return
	}

	// Add the new match to the database
	// This will also update the LP history and league entries
	if err := b.storage.AddMatch(summoner.RiotSummonerID, newMatch, currentRankInfo.LeaguePoints); err != nil {
		log.Printf("Error storing match for %s: %v", summoner.Name, err)
		return
	}

	//get lp_change from last entry from lp_history table based on summoner_id
	lpChange, err := b.storage.GetLastLPChange(summonerID)
	if err != nil {
		log.Printf("Error getting LP change for %s: %v", summoner.Name, err)
		lpChange = 0
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

	log.Printf("New match processed for %s", summoner.Name)
}

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
	leagueOfGraphLink := fmt.Sprintf("https://www.leagueofgraphs.com/match/euw/%s", match.MatchID)

	// Create the embed
	embed := &dg.MessageEmbed{
		Title: fmt.Sprintf("[%s (%s LP)](%s)", summoner.Name, lpChangeStr, leagueOfGraphLink),
		Color: embedColor,
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
