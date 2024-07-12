package updateaccount

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
)

func getAllChoices(guildID string) []*discordgo.ApplicationCommandOptionChoice {
	logger.Log.Info("Getting all choices for account select dropdown")
	var accounts []models.Account
	database.DB.Where("guild_id = ?", guildID).Find(&accounts)
	choices := make([]*discordgo.ApplicationCommandOptionChoice, len(accounts))
	for i, account := range accounts {
		choices[i] = &discordgo.ApplicationCommandOptionChoice{
			Name:  account.Title,
			Value: account.ID,
		}
	}
	return choices
}

func RegisterCommand(s *discordgo.Session, guildID string) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "updateaccount",
			Description: "Update the SSO cookie for an account",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionType(discordgo.InteractionApplicationCommandAutocomplete),
					Name:        "account",
					Description: "The title of the account",
					Required:    true,
					Choices:     getAllChoices(guildID),
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "sso_cookie",
					Description: "The new SSO cookie for the account",
					Required:    true,
				},
			},
		},
	}

	existingCommands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	var existingCommand *discordgo.ApplicationCommand
	for _, command := range existingCommands {
		if command.Name == "updateaccount" {
			existingCommand = command
			break
		}
	}

	newCommand := commands[0]

	if existingCommand != nil {
		logger.Log.Info("Updating updateaccount command")
		_, err := s.ApplicationCommandEdit(s.State.User.ID, guildID, existingCommand.ID, newCommand)
		if err != nil {
			logger.Log.WithError(err).Error("Error updating updateaccount command")
			return
		}
	} else {
		logger.Log.Info("Creating updateaccount command")
		_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, newCommand)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating updateaccount command")
			return
		}
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, command := range commands {
		logger.Log.Infof("Deleting command %s", command.Name)
		err := s.ApplicationCommandDelete(s.State.User.ID, guildID, command.ID)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error deleting command %s ", command.Name)
			return
		}
	}
}

func CommandUpdateAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to defer interaction response")
		return
	}

	userID := i.Member.User.ID
	guildID := i.GuildID
	accountId := i.ApplicationCommandData().Options[0].IntValue()
	newSSOCookie := i.ApplicationCommandData().Options[1].StringValue()

	var account models.Account
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	result := tx.Where("user_id = ? AND id = ? AND guild_id = ?", userID, accountId, guildID).First(&account)
	if result.Error != nil {
		tx.Rollback()
		logger.Log.WithError(result.Error).Error("Error retrieving account")
		sendFollowUpMessage(s, i, "Account does not exist")
		return
	}

	logger.Log.Infof("Verifying new SSO cookie for account %s ", account.Title)
	if !services.VerifySSOCookie(newSSOCookie) {
		tx.Rollback()
		logger.Log.Warn("Invalid new SSO cookie provided")
		sendFollowUpMessage(s, i, "Invalid new SSO cookie")
		return
	}
	logger.Log.Info("New SSO cookie verified successfully")

	account.SSOCookie = newSSOCookie
	account.IsExpiredCookie = false
	account.LastCookieNotification = 0

	newStatus, err := services.CheckAccount(newSSOCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error checking account status with new SSO cookie")
		newStatus = models.StatusUnknown
	}
	account.LastStatus = newStatus
	logger.Log.Infof("Updated account status to %s", newStatus)

	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		logger.Log.WithError(err).Error("Error saving updated account")
		sendFollowUpMessage(s, i, "Error updating account. Please try again.")
		return
	}

	tx.Commit()

	sendFollowUpMessage(s, i, fmt.Sprintf("Account SSO cookie updated. New status: %s", newStatus))
}
func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send follow-up message")
	}
}
