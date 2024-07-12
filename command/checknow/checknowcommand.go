package checknow

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"time"
)

func RegisterCommand(s *discordgo.Session, guildID string) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "checknow",
			Description: "Immediately check the status of a specific account",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionType(discordgo.InteractionApplicationCommandAutocomplete),
					Name:        "account",
					Description: "The title of the account to check",
					Required:    true,
					Choices:     getAllChoices(guildID),
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
		if command.Name == "checknow" {
			existingCommand = command
			break
		}
	}

	newCommand := commands[0]

	if existingCommand != nil {
		logger.Log.Info("Updating checknow command")
		_, err = s.ApplicationCommandEdit(s.State.User.ID, guildID, existingCommand.ID, newCommand)
		if err != nil {
			logger.Log.WithError(err).Error("Error updating checknow command")
			return
		}
	} else {
		logger.Log.Info("Creating checknow command")
		_, err = s.ApplicationCommandCreate(s.State.User.ID, guildID, newCommand)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating checknow command")
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
		if command.Name == "checknow" {
			logger.Log.Infof("Deleting command %s", command.Name)
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, command.ID)
			if err != nil {
				logger.Log.WithError(err).Errorf("Error deleting command %s", command.Name)
				return
			}
		}
	}
}

func CommandCheckNow(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	accountId := i.ApplicationCommandData().Options[0].IntValue()

	var account models.Account
	result := database.DB.Where("user_id = ? AND id = ?", userID, accountId).First(&account)
	if result.Error != nil {
		sendFollowUpMessage(s, i, "Account does not exist or you don't have permission to check it.")
		return
	}

	status, err := services.CheckAccount(account.SSOCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error checking account status")
		sendFollowUpMessage(s, i, "Error checking account status. Please try again later.")
		return
	}

	account.LastStatus = status
	account.LastCheck = time.Now().Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update account after check")
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, status),
		Description: fmt.Sprintf("The current status of account %s is: %s", account.Title, status),
		Color:       services.GetColorForStatus(status, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	sendFollowUpMessage(s, i, "", embed)
}

func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string, embed ...*discordgo.MessageEmbed) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Embeds:  embed,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send follow-up message")
	}
}
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
