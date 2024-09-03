package setcaptchaservice

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"strings"
)

func CommandSetCaptchaService(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	return event.CreateModal(discord.NewModalCreateBuilder().
		SetCustomID("set_captcha_service_modal").
		SetTitle("Set EZ-Captcha API Key").
		AddActionRow(discord.TextInputComponent{
			CustomID:    "api_key",
			Label:       "Enter your EZ-Captcha API key",
			Style:       discord.TextInputStyleShort,
			Placeholder: "Leave blank to use bot's default key",
			Required:    false,
		}).
		Build())
}

func HandleModalSubmit(client bot.Client, event *events.ModalSubmitInteractionCreate) error {
	data := event.Data

	apiKey := strings.TrimSpace(data.Text("api_key"))

	userID := event.User().ID

	if apiKey != "" {
		isValid, err := services.ValidateCaptchaKey(apiKey)
		if err != nil {
			logger.Log.WithError(err).Error("Error validating captcha key")
			return respondToInteraction(event, "Error validating the EZ-Captcha API key. Please try again.")
		}
		if !isValid {
			return respondToInteraction(event, "The provided EZ-Captcha API key is invalid. Please check and try again.")
		}
	}

	err := services.SetUserCaptchaKey(userID.String(), apiKey)
	if err != nil {
		logger.Log.WithError(err).Error("Error setting user captcha key")
		return respondToInteraction(event, "Error setting EZ-Captcha API key. Please try again.")
	}

	message := "Your EZ-Captcha API key has been updated for all your accounts."
	if apiKey == "" {
		message += " The bot's default API key will be used. Your check interval and notification settings have been reset to default values."
	} else {
		message += " Your custom API key has been set. You now have access to more frequent checks and notifications."
	}

	return respondToInteraction(event, message)
}

func respondToInteraction(event *events.ModalSubmitInteractionCreate, message string) error {
	return event.CreateMessage(discord.MessageCreate{
		Content: message,
		Flags:   discord.MessageFlagEphemeral,
	})
}
