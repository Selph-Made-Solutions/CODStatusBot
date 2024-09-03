package setcheckinterval

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"strconv"
	"strings"
)

func CommandSetCheckInterval(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	userSettings, err := services.GetUserSettings(userID.String(), installType)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		return respondToInteraction(event, "Error fetching your settings. Please try again.")
	}

	if userSettings.CaptchaAPIKey == "" {
		return respondToInteraction(event, "You need to set your own EZ-Captcha API key using the /setcaptchaservice command before you can modify these settings.")
	}

	return event.CreateModal(discord.NewModalCreateBuilder().
		SetCustomID("set_check_interval_modal").
		SetTitle("Set User Preferences").
		AddActionRow(discord.TextInputComponent{
			CustomID:    "check_interval",
			Label:       "Check Interval (minutes)",
			Style:       discord.TextInputStyleShort,
			Placeholder: "Enter a number between 1 and 1440 (24 hours)",
			Required:    false,
			Value:       strconv.Itoa(userSettings.CheckInterval),
		}).
		AddActionRow(discord.TextInputComponent{
			CustomID:    "notification_interval",
			Label:       "Notification Interval (hours)",
			Style:       discord.TextInputStyleShort,
			Placeholder: "Enter a number between 1 and 24",
			Required:    false,
			Value:       fmt.Sprintf("%.0f", userSettings.NotificationInterval),
		}).
		AddActionRow(discord.TextInputComponent{
			CustomID:    "notification_type",
			Label:       "Notification Type (channel or dm)",
			Style:       discord.TextInputStyleShort,
			Placeholder: "Enter 'channel' or 'dm'",
			Required:    false,
			Value:       userSettings.NotificationType,
		}).
		Build())
}

func HandleModalSubmit(client bot.Client, event *events.ModalSubmitInteractionCreate) error {
	data := event.Data

	userID := event.User().ID

	userSettings, err := services.GetUserSettings(userID.String(), models.InstallTypeUser)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		return respondToInteraction(event, "Error fetching your settings. Please try again.")
	}

	defaultSettings, err := services.GetDefaultSettings()
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching default settings")
		return respondToInteraction(event, "Error fetching default settings. Please try again.")
	}

	checkInterval := data.Text("check_interval")
	notificationInterval := data.Text("notification_interval")
	notificationType := data.Text("notification_type")

	if checkInterval == "" {
		userSettings.CheckInterval = defaultSettings.CheckInterval
	} else {
		interval, err := strconv.Atoi(checkInterval)
		if err != nil || interval < 1 || interval > 1440 {
			return respondToInteraction(event, "Invalid check interval. Please enter a number between 1 and 1440.")
		}
		userSettings.CheckInterval = interval
	}

	if notificationInterval == "" {
		userSettings.NotificationInterval = defaultSettings.NotificationInterval
	} else {
		interval, err := strconv.ParseFloat(notificationInterval, 64)
		if err != nil || interval < 1 || interval > 24 {
			return respondToInteraction(event, "Invalid notification interval. Please enter a number between 1 and 24.")
		}
		userSettings.NotificationInterval = interval
	}

	if notificationType == "" {
		userSettings.NotificationType = defaultSettings.NotificationType
	} else {
		notificationType = strings.ToLower(notificationType)
		if notificationType != "channel" && notificationType != "dm" {
			return respondToInteraction(event, "Invalid notification type. Please enter 'channel' or 'dm'.")
		}
		userSettings.NotificationType = notificationType
	}

	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return respondToInteraction(event, "Error updating your settings. Please try again.")
	}

	result := database.DB.Model(&models.Account{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"notification_type": userSettings.NotificationType,
		})

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error updating user accounts")
		return respondToInteraction(event, "Error updating accounts with new settings. Please try again.")
	}

	logger.Log.Infof("Updated %d accounts for user %s", result.RowsAffected, userID)

	message := fmt.Sprintf("Your preferences have been updated:\n"+
		"Check Interval: %d minutes\n"+
		"Notification Interval: %.1f hours\n"+
		"Notification Type: %s\n\n"+
		"These settings will be used for all your account checks and notifications.",
		userSettings.CheckInterval, userSettings.NotificationInterval, userSettings.NotificationType)

	return respondToInteraction(event, message)
}

func respondToInteraction(event interface{}, message string) error {
	switch e := event.(type) {
	case *events.ApplicationCommandInteractionCreate:
		return e.CreateMessage(discord.MessageCreate{
			Content: message,
			Flags:   discord.MessageFlagEphemeral,
		})
	case *events.ModalSubmitInteractionCreate:
		return e.CreateMessage(discord.MessageCreate{
			Content: message,
			Flags:   discord.MessageFlagEphemeral,
		})
	default:
		return fmt.Errorf("unsupported event type")
	}
}
