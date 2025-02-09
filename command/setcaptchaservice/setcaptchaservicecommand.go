package setcaptchaservice

import (
	"fmt"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bradselph/CODStatusBot/utils"
	"github.com/bwmarrin/discordgo"
)

var providerLabels = map[string]string{
	"capsolver": "Capsolver",
	"ezcaptcha": "EZCaptcha",
	"2captcha":  "2Captcha",
}

func CommandSetCaptchaService(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cfg := configuration.Get()
	var components []discordgo.MessageComponent

	if cfg.CaptchaService.Capsolver.Enabled {
		components = append(components, createProviderButton("capsolver"))
	}

	if cfg.CaptchaService.EZCaptcha.Enabled {
		components = append(components, createProviderButton("ezcaptcha"))
	}

	if cfg.CaptchaService.TwoCaptcha.Enabled {
		components = append(components, createProviderButton("2captcha"))
	}

	components = append(components, discordgo.Button{
		Label:    "Remove API Key",
		Style:    discordgo.DangerButton,
		CustomID: "set_captcha_remove",
	})

	if len(components) == 1 {
		respondToInteraction(s, i, "No captcha services are currently enabled. Please contact the bot administrator.")
		return
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select a captcha service provider:",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: components},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding with service selection")
	}
}

func HandleCaptchaServiceSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if customID == "set_captcha_remove" {
		handleAPIKeyRemoval(s, i)
		return
	}

	provider := strings.TrimPrefix(customID, "set_captcha_")
	if _, ok := providerLabels[provider]; !ok {
		respondToInteraction(s, i, "Invalid service selection")
		return
	}

	showAPIKeyModal(s, i, provider)
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	provider := strings.TrimPrefix(data.CustomID, "set_captcha_service_modal_")

	userID, err := services.GetUserID(i)
	if err != nil {
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	apiKey := getAPIKeyFromModal(data)
	if err := validateAndSaveAPIKey(s, i, userID, provider, apiKey); err != nil {
		respondToInteraction(s, i, fmt.Sprintf("Error: %v", err))
		return
	}
}

func createProviderButton(provider string) discordgo.Button {
	return discordgo.Button{
		Label:    providerLabels[provider],
		Style:    discordgo.PrimaryButton,
		CustomID: fmt.Sprintf("set_captcha_%s", provider),
	}
}

func handleAPIKeyRemoval(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := services.GetUserID(i)
	if err != nil {
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	if err := services.RemoveCaptchaKey(userID); err != nil {
		respondToInteraction(s, i, "Error removing API key. Please try again.")
		return
	}

	respondToInteraction(s, i, "Your API key has been removed. The bot's default API key will be used. Your check interval and notification settings have been reset to default values.")
}

func showAPIKeyModal(s *discordgo.Session, i *discordgo.InteractionCreate, provider string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: fmt.Sprintf("set_captcha_service_modal_%s", provider),
			Title:    fmt.Sprintf("Set %s API Key", providerLabels[provider]),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "api_key",
							Label:       fmt.Sprintf("Enter your %s API key", providerLabels[provider]),
							Style:       discordgo.TextInputShort,
							Placeholder: "Enter your API key",
							Required:    true,
						},
					},
				},
			},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error showing API key modal")
	}
}

func getAPIKeyFromModal(data discordgo.ModalSubmitInteractionData) string {
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok && textInput.CustomID == "api_key" {
					return utils.SanitizeInput(strings.TrimSpace(textInput.Value))
				}
			}
		}
	}
	return ""
}

func validateAndSaveAPIKey(s *discordgo.Session, i *discordgo.InteractionCreate, userID, provider, apiKey string) error {
	isValid, balance, err := services.ValidateCaptchaKey(apiKey, provider)
	if err != nil {
		return fmt.Errorf("error validating the %s API key: %v", provider, err)
	}
	if !isValid {
		return fmt.Errorf("the provided %s API key is invalid", provider)
	}

	settings := models.UserSettings{UserID: userID}
	if err := database.DB.Where("user_id = ?", userID).FirstOrCreate(&settings).Error; err != nil {
		return fmt.Errorf("error updating settings")
	}

	cfg := configuration.Get()
	settings.PreferredCaptchaProvider = provider
	settings.CheckInterval = cfg.Intervals.Check
	settings.NotificationInterval = cfg.Intervals.Notification
	settings.CustomSettings = true
	settings.CaptchaBalance = balance
	settings.LastBalanceCheck = time.Now()

	updateAPIKeys(&settings, provider, apiKey)

	if err := database.DB.Save(&settings).Error; err != nil {
		return fmt.Errorf("error saving settings")
	}

	respondToInteraction(s, i, fmt.Sprintf(
		"Your %s API key has been set successfully. Your current balance is %.2f points. You now have access to faster check intervals and no rate limits!",
		providerLabels[provider], balance))

	return nil
}

func updateAPIKeys(settings *models.UserSettings, provider, apiKey string) {
	settings.CapSolverAPIKey = ""
	settings.EZCaptchaAPIKey = ""
	settings.TwoCaptchaAPIKey = ""

	switch provider {
	case "capsolver":
		settings.CapSolverAPIKey = apiKey
	case "ezcaptcha":
		settings.EZCaptchaAPIKey = apiKey
	case "2captcha":
		settings.TwoCaptchaAPIKey = apiKey
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
