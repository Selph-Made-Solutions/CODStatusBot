package setpreference

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

func CommandSetPreference(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	return event.CreateMessage(discord.MessageCreate{
		Content: "Select your preferred notification type:",
		Flags:   discord.MessageFlagEphemeral,
		Components: []discord.MessageComponent{
			discord.ActionRowComponent{
				Components: []discord.MessageComponent{
					discord.ButtonComponent{
						Label:    "Channel",
						Style:    discord.ButtonStylePrimary,
						CustomID: "set_preference_channel",
					},
					discord.ButtonComponent{
						Label:    "Direct Message",
						Style:    discord.ButtonStylePrimary,
						CustomID: "set_preference_dm",
					},
				},
			},
		},
	})
}

func HandlePreferenceSelection(client bot.Client, event *events.ComponentInteractionCreate) error {
	customID := event.Data.CustomID()
	var preferenceType string

	switch customID {
	case "set_preference_channel":
		preferenceType = "channel"
	case "set_preference_dm":
		preferenceType = "dm"
	default:
		return respondToInteraction(event, "Invalid preference type. Please try again.")
	}

	userID := event.User().ID

	result := database.DB.Model(&models.Account{}).
		Where("user_id = ?", userID).
		Update("notification_type", preferenceType)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error updating user accounts")
		return respondToInteraction(event, "Error setting preference. Please try again.")
	}

	logger.Log.Infof("Updated %d accounts for user %s", result.RowsAffected, userID)

	message := "Your notification preference has been updated for all your accounts. "
	if preferenceType == "channel" {
		message += "You will now receive notifications in the channel."
	} else {
		message += "You will now receive notifications via direct message."
	}

	return respondToInteraction(event, message)
}

func respondToInteraction(event interface{}, message string) error {
	switch e := event.(type) {
	case *events.ComponentInteractionCreate:
		return e.UpdateMessage(discord.MessageUpdate{
			Content:    discord.NewValueOptional(message),
			Components: []discord.MessageComponent{},
			Flags:      discord.MessageFlagEphemeral,
		})
	case *events.ApplicationCommandInteractionCreate:
		return e.CreateMessage(discord.MessageCreate{
			Content: message,
			Flags:   discord.MessageFlagEphemeral,
		})
	default:
		return nil
	}
}
