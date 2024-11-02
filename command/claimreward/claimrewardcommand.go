package claimreward

import (
	"fmt"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bradselph/CODStatusBot/utils"
	"github.com/bwmarrin/discordgo"
)

func CommandClaimReward(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := getUserID(i)
	if userID == "" {
		logger.Log.Error("Could not determine user ID")
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
		respondToInteraction(s, i, "You don't have any monitored accounts to claim rewards for.")
		return
	}

	var availableCodes []models.RewardCode
	if err := database.DB.Where("active = ? AND expires_at > ?", true, time.Now()).Find(&availableCodes).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching available reward codes")
	}

	var components []discordgo.MessageComponent

	components = append(components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Enter Code Manually",
				Style:    discordgo.PrimaryButton,
				CustomID: "claim_reward_manual",
			},
		},
	})

	if len(availableCodes) > 0 {
		var codeButtons []discordgo.MessageComponent
		for _, code := range availableCodes {
			if !isCodeUsedByUser(code, userID) {
				codeButtons = append(codeButtons, discordgo.Button{
					Label:    fmt.Sprintf("Claim: %s", code.Description),
					Style:    discordgo.SuccessButton,
					CustomID: fmt.Sprintf("claim_reward_preset_%s", code.Code),
				})

				if len(codeButtons) >= 4 {
					break
				}
			}
		}

		if len(codeButtons) > 0 {
			components = append(components, discordgo.ActionsRow{
				Components: codeButtons,
			})
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Choose how you'd like to claim a reward:",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding with reward options")
	}
}

func HandleRewardChoice(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if customID == "claim_reward_manual" {
		showClaimModal(s, i)
		return
	}

	if strings.HasPrefix(customID, "claim_reward_preset_") {
		code := strings.TrimPrefix(customID, "claim_reward_preset_")
		showAccountSelection(s, i, code)
	}
}

func showClaimModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "claim_reward_modal",
			Title:    "Enter Reward Code",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "reward_code",
							Label:       "Reward Code",
							Style:       discordgo.TextInputShort,
							Placeholder: "Enter your reward code",
							Required:    true,
							MinLength:   1,
							MaxLength:   50,
						},
					},
				},
			},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error showing claim modal")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	var code string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok && textInput.CustomID == "reward_code" {
					code = utils.SanitizeInput(strings.TrimSpace(textInput.Value))
				}
			}
		}
	}

	if code == "" {
		respondToInteraction(s, i, "Please enter a valid reward code.")
		return
	}

	showAccountSelection(s, i, code)
}

func showAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate, code string) {
	userID := getUserID(i)
	var accounts []models.Account
	if err := database.DB.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	var components []discordgo.MessageComponent
	var currentRow []discordgo.MessageComponent

	for _, account := range accounts {
		currentRow = append(currentRow, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("claim_account_%s_%d", code, account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	var rewardCode models.RewardCode
	isSingleUse := false
	if err := database.DB.Where("code = ?", code).First(&rewardCode).Error; err == nil {
		isSingleUse = rewardCode.SingleUse
	}

	if !isSingleUse && len(currentRow) < 5 {
		currentRow = append(currentRow, discordgo.Button{
			Label:    "Claim for All Accounts",
			Style:    discordgo.SuccessButton,
			CustomID: fmt.Sprintf("claim_all_%s", code),
		})
	}

	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	message := "Select an account to claim the reward for:"
	if isSingleUse {
		message += "\n⚠️ This is a single-use code and can only be claimed for one account."
	}

	var err error
	if i.Type == discordgo.InteractionModalSubmit {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content:    message,
				Components: components,
				Flags:      discordgo.MessageFlagsEphemeral,
			},
		})
	} else {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    message,
				Components: components,
				Flags:      discordgo.MessageFlagsEphemeral,
			},
		})
	}

	if err != nil {
		logger.Log.WithError(err).Error("Error showing account selection")
	}
}

func HandleClaimSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")

	if len(parts) < 3 {
		logger.Log.Error("Invalid custom ID format")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	code := parts[2]
	userID := getUserID(i)

	if strings.HasPrefix(customID, "claim_all_") {
		handleClaimAll(s, i, code, userID)
		return
	}

	if len(parts) < 4 {
		logger.Log.Error("Invalid custom ID format for single account claim")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	accountID := parts[3]
	handleSingleClaim(s, i, code, accountID, userID)
}

func handleClaimAll(s *discordgo.Session, i *discordgo.InteractionCreate, code, userID string) {
	var accounts []models.Account
	if err := database.DB.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	var successCount int
	var failedAccounts []string
	var firstResponse string

	for _, account := range accounts {
		result, err := services.RedeemCode(account.SSOCookie, code)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to redeem code for account %s", account.Title)
			failedAccounts = append(failedAccounts, account.Title)
			if firstResponse == "" {
				firstResponse = err.Error()
			}
			continue
		}
		successCount++
		if firstResponse == "" {
			firstResponse = result
		}
	}

	updateCodeUsage(code, userID)

	var message string
	if successCount > 0 {
		message = fmt.Sprintf("Successfully claimed reward for %d account(s).\nFirst response: %s", successCount, firstResponse)
		if len(failedAccounts) > 0 {
			message += fmt.Sprintf("\n\nFailed to claim for: %s", strings.Join(failedAccounts, ", "))
		}
	} else {
		message = fmt.Sprintf("Failed to claim reward for any accounts.\nError: %s", firstResponse)
	}

	respondToInteraction(s, i, message)
}

func handleSingleClaim(s *discordgo.Session, i *discordgo.InteractionCreate, code, accountID, userID string) {
	var account models.Account
	if err := database.DB.First(&account, accountID).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching account")
		respondToInteraction(s, i, "Error fetching account details. Please try again.")
		return
	}

	if account.UserID != userID {
		respondToInteraction(s, i, "You don't have permission to claim rewards for this account.")
		return
	}

	result, err := services.RedeemCode(account.SSOCookie, code)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to redeem code")
		respondToInteraction(s, i, fmt.Sprintf("Failed to claim reward: %v", err))
		return
	}

	updateCodeUsage(code, userID)

	respondToInteraction(s, i, fmt.Sprintf("Successfully claimed reward for %s!\n%s", account.Title, result))
}

func updateCodeUsage(code, userID string) {
	var rewardCode models.RewardCode
	if err := database.DB.Where("code = ?", code).First(&rewardCode).Error; err != nil {
		return
	}

	if rewardCode.SingleUse {
		rewardCode.Active = false
	}

	rewardCode.UsedBy = append(rewardCode.UsedBy, userID)
	database.DB.Save(&rewardCode)
}

func isCodeUsedByUser(code models.RewardCode, userID string) bool {
	for _, usedBy := range code.UsedBy {
		if usedBy == userID {
			return true
		}
	}
	return false
}

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
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
