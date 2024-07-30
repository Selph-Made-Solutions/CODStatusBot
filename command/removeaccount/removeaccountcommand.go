package removeaccount

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strconv"
	"strings"
	"unicode"
)

func sanitizeInput(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, input)
}

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "removeaccount",
		Description: "Remove a monitored account",
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating removeaccount command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, cmd := range commands {
		if cmd.Name == "removeaccount" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting removeaccount command")
			}
			return
		}
	}
}

func CommandRemoveAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts to remove.")
		return
	}

	options := make([]discordgo.SelectMenuOption, len(accounts))
	for index, account := range accounts {
		options[index] = discordgo.SelectMenuOption{
			Label:       account.Title,
			Value:       strconv.FormatUint(uint64(account.ID), 10),
			Description: fmt.Sprintf("Status: %s, Guild: %s", account.LastStatus, account.GuildID),
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select an account to remove:",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    "remove_account_select",
							Placeholder: "Choose an account",
							Options:     options,
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) == 0 {
		respondToInteraction(s, i, "No account selected. Please try again.")
		return
	}

	accountID, err := strconv.ParseUint(data.Values[0], 10, 64)
	if err != nil {
		logger.Log.WithError(err).Error("Error converting account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to remove it.")
		return
	}

	// Show confirmation modal
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: fmt.Sprintf("remove_account_modal_%d", accountID),
			Title:    "Confirm Account Removal",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "confirmation",
							Label:       "Type 'CONFIRM' to remove this account",
							Style:       discordgo.TextInputShort,
							Placeholder: "CONFIRM",
							Required:    true,
							MinLength:   7,
							MaxLength:   7,
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error showing confirmation modal")
		respondToInteraction(s, i, "An error occurred. Please try again.")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	var confirmation string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok {
					if textInput.CustomID == "confirmation" {
						confirmation = strings.TrimSpace(textInput.Value)
					}
				}
			}
		}
	}

	if confirmation != "CONFIRM" {
		respondToInteraction(s, i, "Account removal cancelled. The confirmation was not correct.")
		return
	}

	// Extract the account ID from the modal's custom ID
	parts := strings.Split(data.CustomID, "_")
	if len(parts) != 3 {
		logger.Log.Error("Invalid custom ID format")
		respondToInteraction(s, i, "An error occurred. Please try again.")
		return
	}

	accountID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "An error occurred. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to remove it.")
		return
	}

	// Start a transaction
	tx := database.DB.Begin()

	// Delete associated bans
	if err := tx.Where("account_id = ?", account.ID).Delete(&models.Ban{}).Error; err != nil {
		tx.Rollback()
		logger.Log.WithError(err).Error("Error deleting associated bans")
		respondToInteraction(s, i, "Error removing account. Please try again.")
		return
	}

	// Delete the account
	if err := tx.Delete(&account).Error; err != nil {
		tx.Rollback()
		logger.Log.WithError(err).Error("Error deleting account")
		respondToInteraction(s, i, "Error removing account. Please try again.")
		return
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		logger.Log.WithError(err).Error("Error committing transaction")
		respondToInteraction(s, i, "Error removing account. Please try again.")
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Account '%s' has been successfully removed from the database.", account.Title))
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
