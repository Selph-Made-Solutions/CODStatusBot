package setcaptchaservice

import (
	"strings"

	"CODStatusBot/logger"
	"CODStatusBot/services"
	"CODStatusBot/utils"
	"github.com/bwmarrin/discordgo"
)

func CommandSetCaptchaService(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "set_captcha_service_modal",
			Title:    "Set EZ-Captcha API Key",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "api_key",
							Label:       "Enter your EZ-Captcha API key",
							Style:       discordgo.TextInputShort,
							Placeholder: "Leave blank to use bot's default key",
							Required:    false,
						},
					},
				},
			},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding with modal")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	var apiKey string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok && textInput.CustomID == "api_key" {
					apiKey = utils.SanitizeInput(strings.TrimSpace(textInput.Value))
				}
			}
		}
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

	// Validate the API key
	if apiKey != "" {
		isValid, balance, err := services.ValidateCaptchaKey(apiKey)
		if err != nil {
			logger.Log.WithError(err).Error("Error validating captcha key")
			respondToInteraction(s, i, "Error validating the EZ-Captcha API key. Please try again.")
			return
		}
		if !isValid {
			respondToInteraction(s, i, "The provided EZ-Captcha API key is invalid. Please check and try again.")
			return
		}
		logger.Log.Infof("Valid captcha key set for user: %s. Balance: %.2f points", userID, balance)
	}

	err := services.SetUserCaptchaKey(userID, apiKey)
	if err != nil {
		logger.Log.WithError(err).Error("Error setting user captcha key")
		respondToInteraction(s, i, "Error setting EZ-Captcha API key. Please try again.")
		return
	}

	message := "Your EZ-Captcha API key has been updated for all your accounts."
	if apiKey == "" {
		message += " The bot's default API key will be used. Your check interval and notification settings have been reset to default values."
	} else {
		message += " Your custom API key has been set. You now have access to more frequent checks and notifications."
	}

	respondToInteraction(s, i, message)
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
