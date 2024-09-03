package globalannouncement

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"os"
	"time"
)

func CommandGlobalAnnouncement(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		return respondToInteraction(event, "Error: Developer ID not configured.")
	}

	userID := event.User().ID

	if userID.String() != developerID {
		logger.Log.Warnf("Unauthorized user %s attempted to use global announcement command", userID)
		return respondToInteraction(event, "You don't have permission to use this command. Only the bot developer can send global announcements.")
	}

	successCount, failCount, err := SendAnnouncementToAllUsers(client)
	if err != nil {
		logger.Log.WithError(err).Error("Error occurred while sending global announcement")
		return respondToInteraction(event, fmt.Sprintf("An error occurred while sending the global announcement. %d messages sent successfully, %d failed.", successCount, failCount))
	}

	return respondToInteraction(event, fmt.Sprintf("Global announcement sent successfully to %d users. %d users could not be reached.", successCount, failCount))
}

func SendAnnouncementToAllUsers(client bot.Client) (int, int, error) {
	var users []models.UserSettings
	if err := database.DB.Find(&users).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching all users")
		return 0, 0, err
	}

	successCount := 0
	failCount := 0

	for _, user := range users {
		err := SendGlobalAnnouncement(client, user.UserID)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send announcement to user %s", user.UserID)
			failCount++
		} else {
			successCount++
		}
	}

	return successCount, failCount, nil
}

func SendGlobalAnnouncement(client bot.Client, userID string) error {
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings for global announcement")
		return result.Error
	}

	if !userSettings.HasSeenAnnouncement {
		var channelID discord.Snowflake
		var err error

		if userSettings.NotificationType == "dm" {
			channel, err := client.Rest().CreateDM(discord.Snowflake(userID))
			if err != nil {
				logger.Log.WithError(err).Error("Error creating DM channel for global announcement")
				return err
			}
			channelID = channel.ID
		} else {
			var account models.Account
			if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err != nil {
				logger.Log.WithError(err).Error("Error finding recent channel for user")
				return err
			}
			channelID = discord.Snowflake(account.ChannelID)
		}

		announcementEmbed := createAnnouncementEmbed()

		_, err = client.Rest().CreateMessage(channelID, discord.NewMessageCreateBuilder().
			SetEmbeds(announcementEmbed).
			Build())
		if err != nil {
			logger.Log.WithError(err).Error("Error sending global announcement")
			return err
		}

		userSettings.HasSeenAnnouncement = true
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Error("Error updating user settings after sending global announcement")
			return err
		}
	}

	return nil
}

func createAnnouncementEmbed() discord.Embed {
	return discord.NewEmbedBuilder().
		SetTitle("Important Update: Changes to COD Status Bot").
		SetDescription("Due to high demand, we've reached our limit of free EZCaptcha tokens. To ensure continued functionality, we're introducing some changes:").
		SetColor(0xFFD700).
		AddField("What's Changing", "• The check ban feature now requires users to provide their own EZCaptcha API key.\n"+
			"• Without an API key, the bot's check ban functionality will be limited.", false).
		AddField("How to Get Your Own API Key", "1. Sign up at [EZ-Captcha](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) using our referral link.\n"+
			"2. Request a free trial of 10,000 tokens.\n"+
			"3. Use the `/setcaptchaservice` command to set your API key in the bot.", false).
		AddField("Benefits of Using Your Own API Key", "• Uninterrupted access to the check ban feature\n"+
			"• Ability to customize check intervals\n"+
			"• Support the bot's development through our referral program", false).
		AddField("Next Steps", "1. Obtain your API key as soon as possible.\n"+
			"2. Set up your key using the `/setcaptchaservice` command.\n"+
			"3. Adjust your check interval preferences if desired.", false).
		AddField("Our Commitment", "We're actively exploring ways to maintain a free tier for all users. Your support through the referral program directly contributes to this goal.", false).
		SetFooter("Thank you for your understanding and continued support!", "").
		SetTimestamp(time.Now()).
		Build()
}

func respondToInteraction(event *events.ApplicationCommandInteractionCreate, message string) error {
	return event.CreateMessage(discord.MessageCreate{
		Content: message,
		Flags:   discord.MessageFlagEphemeral,
	})
}
