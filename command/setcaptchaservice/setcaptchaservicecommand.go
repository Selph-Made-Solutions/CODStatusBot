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
		Description: "Set your preferred captcha service",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "service",
				Description: "The captcha service to use (ezcaptcha or 2captcha)",
				Required:    true,
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
	captchaService := i.ApplicationCommandData().Options[0].StringValue()

	if captchaService != "ezcaptcha" && captchaService != "2captcha" {
		respondToInteraction(s, i, "Invalid captcha service. Please choose either 'ezcaptcha' or '2captcha'.")
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

	result := database.DB.Model(&models.Account{}).
		Where("user_id = ?", userID).
		Update("captcha_service", captchaService)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error updating user accounts")
		respondToInteraction(s, i, "Error setting captcha service. Please try again.")
		return
	}

	logger.Log.Infof("Updated %d accounts for user %s", result.RowsAffected, userID)

	message := "Your captcha service preference has been updated to " + captchaService + " for all your accounts."
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
