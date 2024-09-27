package updateaccount

import (
	"fmt"
	"strconv"
	"strings"

	"CODStatusBot/database"
	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"CODStatusBot/utils"

	"github.com/bwmarrin/discordgo"
)

func CommandUpdateAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := getUserID(i)
	if err != nil {
		handleError(s, i, err)
		return
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		handleError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching user accounts"))
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts to update.")
		return
	}

	// Create buttons for each account
	var components []discordgo.MessageComponent
	var currentRow []discordgo.MessageComponent

	for _, account := range accounts {
		currentRow = append(currentRow, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("update_account_%d", account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	// Add the last row if it is not empty.
	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an account to update:",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		handleError(s, i, errorhandler.NewDiscordError(err, "sending account selection message"))
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "update_account_"))
	if err != nil {
		handleError(s, i, errorhandler.NewValidationError(err, "account ID"))
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: fmt.Sprintf("update_account_modal_%d", accountID),
			Title:    "Update Account",
			Components: []discordgo.MessageComponent{
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
		handleError(s, i, errorhandler.NewDiscordError(err, "sending update account modal"))
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	accountIDStr := strings.TrimPrefix(data.CustomID, "update_account_modal_")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		handleError(s, i, errorhandler.NewValidationError(err, "account ID"))
		return
	}

	var newSSOCookie string

	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if v, ok := rowComp.(*discordgo.TextInput); ok && v.CustomID == "new_sso_cookie" {
					newSSOCookie = utils.SanitizeInput(strings.TrimSpace(v.Value))
				}
			}
		}
	}

	if newSSOCookie == "" {
		handleError(s, i, errorhandler.NewValidationError(fmt.Errorf("empty SSO cookie"), "SSO cookie"))
		return
	}

	// Verify the new SSO cookie
	if !services.VerifySSOCookie(newSSOCookie) {
		handleError(s, i, errorhandler.NewValidationError(fmt.Errorf("invalid SSO cookie"), "SSO cookie"))
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		handleError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching account"))
		return
	}

	// Verify that the user owns this account
	userID, err := getUserID(i)
	if err != nil {
		handleError(s, i, err)
		return
	}

	if account.UserID != userID {
		handleError(s, i, errorhandler.NewAuthenticationError(fmt.Errorf("user doesn't own this account")))
		return
	}

	// Get SSO Cookie expiration
	expirationTimestamp, err := services.DecodeSSOCookie(newSSOCookie)
	if err != nil {
		handleError(s, i, errorhandler.NewValidationError(err, "SSO cookie"))
		return
	}

	// Update the account
	account.SSOCookie = newSSOCookie
	account.SSOCookieExpiration = expirationTimestamp
	account.IsExpiredCookie = false
	account.IsCheckDisabled = false // Reset the disabled status
	account.DisabledReason = ""     // Clear the disabled reason
	account.ConsecutiveErrors = 0   // Reset consecutive errors

	services.DBMutex.Lock()
	if err := database.DB.Save(&account).Error; err != nil {
		services.DBMutex.Unlock()
		handleError(s, i, errorhandler.NewDatabaseError(err, "updating account"))
		return
	}
	services.DBMutex.Unlock()

	formattedExpiration := services.FormatExpirationTime(expirationTimestamp)
	message := fmt.Sprintf("Account '%s' has been successfully updated. New SSO cookie will expire in %s. Account checks have been re-enabled.", account.Title, formattedExpiration)
	respondToInteraction(s, i, message)
}

func handleError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	userMsg, _ := errorhandler.HandleError(err)
	respondToInteraction(s, i, userMsg)
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	var err error
	if i.Type == discordgo.InteractionMessageComponent {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    message,
				Components: []discordgo.MessageComponent{},
				Flags:      discordgo.MessageFlagsEphemeral,
			},
		})
	} else {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}

func getUserID(i *discordgo.InteractionCreate) (string, error) {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID, nil
	}
	if i.User != nil {
		return i.User.ID, nil
	}
	return "", errorhandler.NewValidationError(fmt.Errorf("unable to determine user ID"), "user identification")
}
