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

func (b *Bot) formatDailySummary(progress []storage.DailySummonerProgress) []*discordgo.MessageEmbed {
	if len(progress) == 0 {
		return []*discordgo.MessageEmbed{{
			Title:       "Daily Summary",
			Description: "No summoner progress to report today 😔",
			Color:       0x3498db,
		}}
	}

	happiestSummoner := progress[0]
	saddestSummoner := progress[len(progress)-1]

	happiestSummonerLPChange := happiestSummoner.CurrentLP - happiestSummoner.PreviousLP
	saddestSummonerLPChange := saddestSummoner.CurrentLP - saddestSummoner.PreviousLP

	// Create the first embed for best and worst performers
	summaryEmbed := &discordgo.MessageEmbed{
		Title: "Daily Summary",
		Color: 0x3498db,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "**🏆 Happiest summoner**",
				Value: fmt.Sprintf("%s (%+d LP)", happiestSummoner.Name, happiestSummonerLPChange),
			},
			{
				Name:  "**😢 Saddest summoner**",
				Value: fmt.Sprintf("%s (%+d LP)", saddestSummoner.Name, saddestSummonerLPChange),
			},
		},
	}

	// Create the second embed for individual summoner changes
	changesEmbed := &discordgo.MessageEmbed{
		Title:  "Daily Summary - All Summoners",
		Color:  0x3498db,
		Fields: []*discordgo.MessageEmbedField{},
	}

	// Add individual summoner progress
	for _, p := range progress {
		lpChange := p.CurrentLP - p.PreviousLP
		nameField := fmt.Sprintf("**%s**  •  %+dLP (%dW/%dL)", p.Name, lpChange, p.Wins, p.Losses)

		valueField := fmt.Sprintf("-# %s %s • %d LP ➡️ %s %s • %d LP",
			utils.CapitalizeFirst(strings.ToLower(p.PreviousTier)), p.PreviousRank, p.PreviousLP,
			utils.CapitalizeFirst(strings.ToLower(p.CurrentTier)), p.CurrentRank, p.CurrentLP)

		changesEmbed.Fields = append(changesEmbed.Fields, &discordgo.MessageEmbedField{
			Name:  nameField,
			Value: valueField,
		})
	}

	return []*discordgo.MessageEmbed{summaryEmbed, changesEmbed}
}

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

			if parisTime.Hour() == 11 && (parisTime.Minute() == 31) {
				log.Println("Running daily summary")
				b.PublishDailySummary()
				log.Println("Daily summary completed")
				time.Sleep(time.Minute)
			}
		}
	}
}
