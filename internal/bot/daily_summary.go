package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
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
		guildName, err := b.storage.GetGuildName(guild.ID)
		if err != nil {
			log.Printf("Error getting guild's name: %v", err)
		}

		progress, err := b.storage.GetDailySummonerProgress(guild.ID)
		if err != nil {
			log.Printf("Error getting daily summoner progress for guild %s: %v", guildName, err)
			continue
		}

		if len(progress) == 0 {
			continue
		}

		summary := b.formatDailySummary(progress)

		if guild.ChannelID == "" {
			log.Printf("No channel ID set for guild %s", guild.ID)
			continue
		}

		_, err = b.session.ChannelMessageSendEmbed(guild.ChannelID, summary)
		if err != nil {
			log.Printf("Error sending daily summary to guild %s: %v", guild.ID, err)
		}
	}
}

func (b *Bot) formatDailySummary(progress []storage.DailySummonerProgress) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "Daily Summary",
		Description: "",
		Color:       0x3498db,
		Fields:      []*discordgo.MessageEmbedField{},
	}

	if len(progress) == 0 {
		embed.Description = "No summoner progress to report today 😔"
		return embed
	}

	// Sort the progress slice by LP change (descending)
	sort.Slice(progress, func(i, j int) bool {
		return (progress[i].CurrentLP - progress[i].PreviousLP) > (progress[j].CurrentLP - progress[j].PreviousLP)
	})

	// Add best performer
	best := progress[0]
	bestLPChange := best.CurrentLP - best.PreviousLP
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:  "🏆 Happiest summoner",
		Value: fmt.Sprintf("**%s** (%+d LP)", best.Name, bestLPChange),
	})

	// Add worst performer
	worst := progress[len(progress)-1]
	worstLPChange := worst.CurrentLP - worst.PreviousLP
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:  "😢 Saddest summoner",
		Value: fmt.Sprintf("**%s** (%+d LP)", worst.Name, worstLPChange),
	})

	// Add individual summoner progress
	for _, p := range progress {
		lpChange := p.CurrentLP - p.PreviousLP
		fieldValue := fmt.Sprintf("%+d LP (%dW/%dL)\n", lpChange, p.Wins, p.Losses)
		fieldValue += fmt.Sprintf("```%s %s - %d LP ➡️ %s %s - %d LP```",
			utils.CapitalizeFirst(strings.ToLower(p.PreviousTier)), p.PreviousRank, p.PreviousLP,
			utils.CapitalizeFirst(strings.ToLower(p.CurrentTier)), p.CurrentRank, p.CurrentLP)

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  p.Name,
			Value: fieldValue,
		})
	}

	return embed
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

			if parisTime.Hour() == 14 && (parisTime.Minute() >= 15 && parisTime.Minute() < 17) {
				log.Println("Running daily summary")
				b.PublishDailySummary()
				log.Println("Daily summary completed")
				time.Sleep(time.Minute)
			}
		}
	}
}
