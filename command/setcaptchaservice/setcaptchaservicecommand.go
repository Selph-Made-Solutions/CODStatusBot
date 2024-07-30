package setcaptchaservice

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "setcaptchaservice",
		Description: "Set your EZ-Captcha API key",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "api_key",
				Description: "Your EZ-Captcha API key (leave empty to use bot's default key)",
				Required:    false,
			},
		},
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating setcaptchaservice command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, command := range commands {
		if command.Name == "setcaptchaservice" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, command.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting setcaptchaservice command")
			}
			return
		}
	}
}

func CommandSetCaptchaService(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var apiKey string
	if len(options) > 0 {
		apiKey = options[0].StringValue()
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

	// Update all accounts for this user
	result := database.DB.Model(&models.Account{}).
		Where("user_id = ?", userID).
		Update("captcha_api_key", apiKey)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error updating user accounts")
		respondToInteraction(s, i, "Error setting EZ-Captcha API key. Please try again.")
		return
	}

	logger.Log.Infof("Updated %d accounts for user %s", result.RowsAffected, userID)

	message := "Your EZ-Captcha API key has been updated for all your accounts."
	if apiKey == "" {
		message += " The bot's default API key will be used."
	} else {
		message += " Your custom API key has been set."
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
