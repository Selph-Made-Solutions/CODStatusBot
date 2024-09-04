package listaccounts

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"math/rand"
	"time"
)

func CommandListAccounts(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Your Monitored Accounts",
		Description: "Here's a list of all your monitored accounts:",
		Color:       randomColor(),
		Fields:      make([]*discordgo.MessageEmbedField, len(accounts)),
	}

	for i, account := range accounts {
		embed.Fields[i] = &discordgo.MessageEmbedField{
			Name: account.Title,
			Value: fmt.Sprintf("Status: %v\nGuild: %s\nNotification Type: %s",
				account.LastStatus, account.GuildID, account.NotificationType),
			Inline: false,
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}

// Function to generate a random color in 0xRRGGBB format
func randomColor() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano())) // Seed the random number generator
	return r.Intn(0xFFFFFF)                              // Generate a random color in 0xRRGGBB format
}
