package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
)

// TrackMatches initiates match tracking for all summoners in a given guild
func (b *Bot) TrackMatches(guildID string) error {
	summoners, err := b.storage.GetAllSummonersForGuild(guildID)
	if err != nil {
		return fmt.Errorf("error getting summoners: %w", err)
	}

	log.Printf("tracking summoners...")

	if len(summoners) == 0 {
		time.Sleep(2 * time.Minute)
	}

	// Start a goroutine for each summoner to track their matches
	for i, summoner := range summoners {
		go func(s riotapi.Summoner) {
			time.Sleep(time.Duration(i) * 10 * time.Second)
			b.trackSummonerMatches(guildID, s)
		}(summoner)
	}

	return nil
}

// trackSummonerMatches continuously checks for new matches for a specific summoner
func (b *Bot) trackSummonerMatches(guildID string, summoner riotapi.Summoner) {
	for {
		log.Printf("Checking for new ranked solo/duo matches for %s (PUUID: %s)", summoner.Name, summoner.SummonerPUUID)

		storedMatchID, err := b.storage.GetLastMatchID(summoner.SummonerPUUID)
		if err != nil {
			log.Printf("Error getting stored match ID for %s: %v", summoner.Name, err)
			time.Sleep(5 * time.Minute)
			continue
		}

		hasNewMatch, newMatch, err := b.riotClient.GetNewMatchForSummoner(summoner.SummonerPUUID, storedMatchID)
		if err != nil {
			if strings.Contains(err.Error(), "rate limit exceeded") {
				log.Printf("Rate limit exceeded for %s, retrying in 2 minutes", summoner.Name)
				time.Sleep(2 * time.Minute)
			} else {
				log.Printf("Error checking for new match for %s: %v", summoner.Name, err)
				time.Sleep(5 * time.Minute)
			}
			continue
		}

		if hasNewMatch {
			if err := b.storage.AddMatch(summoner.RiotSummonerID, newMatch); err != nil {
				log.Printf("Error storing match for %s: %v", summoner.Name, err)
			} else {
				b.announceNewMatch(guildID, summoner, newMatch)
			}

			// Wait for a longer period after finding a new match
			log.Printf("New match found for %s. Waiting for 10 minutes before next check", summoner.Name)
			time.Sleep(10 * time.Minute)
		} else {
			time.Sleep(5 * time.Minute)
		}
	}
}

// announceNewMatch publish a new match game data into the given channel_id for a guild_id.
func (b *Bot) announceNewMatch(guildID string, summoner riotapi.Summoner, match *riotapi.MatchData) {
	channelID, err := b.storage.GetGuildChannelID(guildID)
	if err != nil {
		log.Printf("Error getting channel ID for guild %s: %v", guildID, err)
		return
	}

	message := fmt.Sprintf("New match for %s:\n"+
		"Champion: %s\n"+
		"Result: %s\n"+
		"KDA: %d/%d/%d\n"+
		"CS: %d",
		summoner.Name, match.ChampionName, match.Result,
		match.Kills, match.Deaths, match.Assists,
		match.TotalMinionsKilled+match.NeutralMinionsKilled)

	_, err = b.session.ChannelMessageSend(channelID, message)
	if err != nil {
		log.Printf("Error sending message to channel %s: %v", channelID, err)
	}
}
