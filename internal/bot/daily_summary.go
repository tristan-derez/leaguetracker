package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/tristan-derez/league-tracker/internal/storage"
	"github.com/tristan-derez/league-tracker/internal/utils"
)

// PublishDailySummary generates and publishes a daily summary for each guild.
// It fetches the progress data for all summoners in a guild and sends
// formatted embed messages to the designated channel in each guild.
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

		embeds := b.formatDailySummary(progress)

		if guild.ChannelID == "" {
			log.Printf("No channel ID set for guild %s", guild.ID)
			continue
		}

		for _, embed := range embeds {
			_, err = b.session.ChannelMessageSendEmbed(guild.ChannelID, embed)
			if err != nil {
				log.Printf("Error sending daily summary embed to guild %s: %v", guild.ID, err)
			}
		}
	}
}

// formatDailySummary creates formatted Discord embeds for the daily summary.
// It takes a slice of DailySummonerProgress and returns two message embeds:
// one for the overall summary and another for detailed changes.
// If no progress data is available, it returns a single embed with a message.
func (b *Bot) formatDailySummary(progress []storage.DailySummonerProgress) []*discordgo.MessageEmbed {
	if len(progress) == 0 {
		return []*discordgo.MessageEmbed{{
			Title:       "Daily Summary",
			Description: "No summoner progress to report today 😔",
			Color:       0x3498db,
		}}
	}

	summaryEmbed := &discordgo.MessageEmbed{
		Title:  "Daily Summary",
		Color:  0x3498db,
		Fields: []*discordgo.MessageEmbedField{},
	}

	happiestsummoner := progress[0]
	summaryEmbed.Fields = append(summaryEmbed.Fields, &discordgo.MessageEmbedField{
		Name:  "**🏆 Happiest summoner**",
		Value: fmt.Sprintf("%s (%+d LP)", happiestsummoner.Name, happiestsummoner.LPChange),
	})

	if len(progress) > 1 {
		saddestSummoner := progress[len(progress)-1]
		summaryEmbed.Fields = append(summaryEmbed.Fields, &discordgo.MessageEmbedField{
			Name:  "**😢 Saddest summoner**",
			Value: fmt.Sprintf("%s (%+d LP)", saddestSummoner.Name, saddestSummoner.LPChange),
		})
	}

	changesEmbed := &discordgo.MessageEmbed{
		Title:  "Daily Summary - Top 10",
		Color:  0x3498db,
		Fields: []*discordgo.MessageEmbedField{},
	}

	for _, p := range progress {
		changesEmbed.Fields = append(changesEmbed.Fields, &discordgo.MessageEmbedField{
			Name: fmt.Sprintf("%s **%s**  •  %+d LP (%dW/%dL)", utils.GetSummaryRankDisplay(p.Rank), p.Name, p.LPChange, p.Wins, p.Losses),
			Value: fmt.Sprintf("%s %s • %d LP ➡️ %s %s • %d LP",
				utils.CapitalizeFirst(strings.ToLower(p.PreviousTier)), p.PreviousRank, p.PreviousLP,
				utils.CapitalizeFirst(strings.ToLower(p.CurrentTier)), p.CurrentRank, p.CurrentLP),
		})
	}

	return []*discordgo.MessageEmbed{summaryEmbed, changesEmbed}
}

// runDailySummaryJob starts a goroutine that runs the daily summary job.
// It checks the time every minute and triggers the PublishDailySummary function
// at 10:55 AM Paris time every day.
func (b *Bot) runDailySummaryJob() {
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

			if parisTime.Hour() == 10 && (parisTime.Minute() == 00) {
				log.Println("Running daily summary")
				b.PublishDailySummary()
				log.Println("Daily summary completed")
				time.Sleep(time.Minute)
			}
		}
	}
}
