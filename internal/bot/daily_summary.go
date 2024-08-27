package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/tristan-derez/league-tracker/internal/storage"
	"github.com/tristan-derez/league-tracker/internal/utils"
)

// PublishDailySummary generates and publishes a daily summary for each guild
func (b *Bot) PublishDailySummary() {
	guilds, err := b.storage.GetAllGuilds()
	if err != nil {
		log.Printf("Error getting guilds: %v", err)
		return
	}
	log.Printf("Retrieved %d guilds for daily summary", len(guilds))

	for _, guild := range guilds {
		log.Printf("Processing guild: %s", guild.ID)
		progress, err := b.storage.GetDailySummonerProgress(guild.ID)
		if err != nil {
			log.Printf("Error getting daily summoner progress for guild %s: %v", guild.ID, err)
			continue
		}

		if len(progress) == 0 {
			log.Printf("No progress to report for guild %s", guild.ID)
			continue
		}
		log.Printf("Found progress for %d summoners in guild %s", len(progress), guild.ID)

		summary := b.formatDailySummary(progress)

		if guild.ChannelID == "" {
			log.Printf("No channel ID set for guild %s", guild.ID)
			continue
		}

		_, err = b.session.ChannelMessageSend(guild.ChannelID, summary)
		if err != nil {
			log.Printf("Error sending daily summary to guild %s: %v", guild.ID, err)
		} else {
			log.Printf("Successfully sent daily summary to guild %s", guild.ID)
		}
	}
}

func (b *Bot) formatDailySummary(progress []storage.DailySummonerProgress) string {
	var summary strings.Builder
	summary.WriteString("**Daily Summary**\n\n")

	if len(progress) == 0 {
		summary.WriteString("No summoner progress to report today 😔")
		return summary.String()
	}

	// Sort the progress slice by LP change (descending)
	sort.Slice(progress, func(i, j int) bool {
		return (progress[i].CurrentLP - progress[i].PreviousLP) > (progress[j].CurrentLP - progress[j].PreviousLP)
	})

	// Add best performer
	best := progress[0]
	bestLPChange := best.CurrentLP - best.PreviousLP
	summary.WriteString(fmt.Sprintf("🏆 Happiest summoner: **%s** (%+d LP)\n", best.Name, bestLPChange))

	// Add worst performer
	worst := progress[len(progress)-1]
	worstLPChange := worst.CurrentLP - worst.PreviousLP
	summary.WriteString(fmt.Sprintf("😢 Saddest summoner: **%s** (%+d LP)\n\n", worst.Name, worstLPChange))

	for _, p := range progress {
		lpChange := p.CurrentLP - p.PreviousLP
		summary.WriteString(fmt.Sprintf("%s: %+d (%dW/%dL)\n", p.Name, lpChange, p.Wins, p.Losses))
		summary.WriteString(fmt.Sprintf("%s %s - %d LP ➡️ %s %s - %d LP\n\n",
			utils.CapitalizeFirst(strings.ToLower(p.PreviousTier)), p.PreviousRank, p.PreviousLP,
			utils.CapitalizeFirst(strings.ToLower(p.CurrentTier)), p.CurrentRank, p.CurrentLP))
	}

	return summary.String()
}

func (b *Bot) runDailySummaryJob() {
	log.Println("Starting daily summary job")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			log.Println("Stopping daily summary job")
			return
		case t := <-ticker.C:
			utcTime := t.UTC()
			parisLocation, err := time.LoadLocation("Europe/Paris")
			if err != nil {
				log.Printf("Error loading Paris time zone: %v", err)
				continue
			}
			parisTime := utcTime.In(parisLocation)

			log.Printf("Ticker triggered at %v Paris time", parisTime)
			if parisTime.Hour() == 13 && parisTime.Minute() == 35 {
				log.Println("Running daily summary")
				b.PublishDailySummary()
				log.Println("Daily summary completed")
				time.Sleep(time.Minute)
			}
		}
	}
}
