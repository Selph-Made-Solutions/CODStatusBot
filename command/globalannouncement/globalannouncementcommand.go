package globalannouncement

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"time"
)

func CommandGlobalAnnouncement(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if the user is the developer
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		respondToInteraction(s, i, "Error: Developer ID not configured.")
		return
	}

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

	if userID != developerID {
		logger.Log.Warnf("Unauthorized user %s attempted to use global announcement command", userID)
		respondToInteraction(s, i, "You don't have permission to use this command. Only the bot developer can send global announcements.")
		return
	}

	// Send the announcement to all users
	successCount, failCount, err := SendAnnouncementToAllUsers(s)
	if err != nil {
		logger.Log.WithError(err).Error("Error occurred while sending global announcement")
		respondToInteraction(s, i, fmt.Sprintf("An error occurred while sending the global announcement. %d messages sent successfully, %d failed.", successCount, failCount))
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Global announcement sent successfully to %d users. %d users could not be reached.", successCount, failCount))
}

func SendAnnouncementToAllUsers(s *discordgo.Session) (int, int, error) {
	var users []models.UserSettings
	if err := database.DB.Find(&users).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching all users")
		return 0, 0, err
	}

	successCount := 0
	failCount := 0

	for _, user := range users {
		err := SendGlobalAnnouncement(s, user.UserID)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send announcement to user %s", user.UserID)
			failCount++
		} else {
			successCount++
		}
	}

	return successCount, failCount, nil
}

func SendGlobalAnnouncement(s *discordgo.Session, userID string) error {
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Errorf("Error getting user settings for user %s", userID)
		return result.Error
	}

	var channelID string
	var err error

	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error creating DM channel for user %s", userID)
			return err
		}
		channelID = channel.ID
	} else {
		// Find the most recent channel used by the user
		var account models.Account
		if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Error finding recent channel for user %s", userID)
			return err
		}
		channelID = account.ChannelID
	}

	announcementEmbed := createAnnouncementEmbed()

	_, err = s.ChannelMessageSendEmbed(channelID, announcementEmbed)
	if err != nil {
		logger.Log.WithError(err).Errorf("Error sending global announcement to user %s", userID)
		return err
	}

	userSettings.HasSeenAnnouncement = true
	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Errorf("Error updating user settings after sending global announcement for user %s", userID)
		return err
	}

	return nil
}

func createAnnouncementEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Important Announcement: Changes to COD Status Bot",
		Description: "Due to the high demand and usage of our bot, we've reached the limit of our free EZCaptcha tokens. " +
			"To continue using the check ban feature, users now need to provide their own EZCaptcha API key.\n\n" +
			"Here's what you need to know:",
		Color: 0xFFD700, // Gold color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "How to Get Your Own API Key",
				Value: "1. Visit our [referral link](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) to sign up for EZCaptcha\n" +
					"2. Request a free trial of 10,000 tokens\n" +
					"3. Use the `/setcaptchaservice` command to set your API key in the bot",
			},
			{
				Name: "Benefits of Using Your Own API Key",
				Value: "• Continue using the check ban feature\n" +
					"• Customize your check intervals\n" +
					"• Support the bot indirectly through our referral program",
			},
			{
				Name:  "Our Commitment",
				Value: "We're working on ways to maintain a free tier for all users. Your support by using our referral link helps us achieve this goal.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Thank you for your understanding and continued support!",
		},
		Timestamp: time.Now().Format(time.RFC3339),
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
