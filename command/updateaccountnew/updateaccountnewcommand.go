package updateaccountnew

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strings"
)

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "updateaccountnew",
		Description: "Update a monitored account's information",
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating updateaccountnew command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, cmd := range commands {
		if cmd.Name == "updateaccountnew" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting updateaccountnew command")
			}
			return
		}
	}
}

func CommandUpdateAccountNew(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := i.Member.User.ID
	guildID := i.GuildID

	var accounts []models.Account
	result := database.DB.Where("user_id = ? AND guild_id = ?", userID, guildID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts to update.")
		return
	}

	accountList := "Your accounts:\n"
	for _, account := range accounts {
		accountList += fmt.Sprintf("• %s (Status: %s)\n", account.Title, account.LastStatus)
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "update_account_modal",
			Title:    "Update Account",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "account_title",
							Label:       "Enter the title of the account to update",
							Style:       discordgo.TextInputShort,
							Placeholder: "Enter the account title",
							Required:    true,
							MinLength:   1,
							MaxLength:   100,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "new_sso_cookie",
							Label:       "Enter the new SSO cookie",
							Style:       discordgo.TextInputParagraph,
							Placeholder: "Enter the new SSO cookie",
							Required:    true,
							MinLength:   1,
							MaxLength:   4000,
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with modal")
		return
	}

	// Send the account list as a follow-up message
	_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: accountList,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error sending follow-up message with account list")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	var accountTitle, newSSOCookie string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok {
					switch textInput.CustomID {
					case "account_title":
						accountTitle = strings.TrimSpace(textInput.Value)
					case "new_sso_cookie":
						newSSOCookie = strings.TrimSpace(textInput.Value)
					}
				}
			}
		}
	}

	if accountTitle == "" || newSSOCookie == "" {
		respondToInteraction(s, i, "Error: Both account title and new SSO cookie must be provided.")
		return
	}

	// Verify the new SSO cookie
	if !services.VerifySSOCookie(newSSOCookie) {
		respondToInteraction(s, i, "Error: The provided SSO cookie is invalid. Please check and try again.")
		return
	}

	var account models.Account
	result := database.DB.Where("title = ? AND user_id = ? AND guild_id = ?", accountTitle, i.Member.User.ID, i.GuildID).First(&account)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to update it.")
		return
	}

	// Update the account
	account.SSOCookie = newSSOCookie
	account.IsExpiredCookie = false // Reset the expired cookie flag

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Error updating account")
		respondToInteraction(s, i, "Error updating account. Please try again.")
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Account '%s' has been successfully updated with the new SSO cookie.", account.Title))
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
