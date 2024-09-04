package accountage

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strconv"
	"strings"
	"time"
)

func CommandAccountAge(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := getUserID(i)
	if userID == "" {
		logger.Log.Error("Failed to get user ID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	accounts, err := getAccounts(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	// Create buttons for each account
	components := createAccountButtons(accounts)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select an account to check its age:",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: components,
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}

	// Log command usage for admin monitoring
	services.IncrementCommandUsage("account_age")
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "account_age_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	account, err := getAccount(accountID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to check its age.")
		return
	}

	if !services.VerifySSOCookie(account.SSOCookie) {
		account.IsExpiredCookie = true
		err = database.DB.Save(&account).Error
		if err != nil {
			logger.Log.WithError(err).Error("Error updating account's cookie status")
		}
		respondToInteraction(s, i, "Invalid SSOCookie. Account's cookie status updated.")
		return
	}

	years, months, days, err := services.CheckAccountAge(account.SSOCookie)
	if err != nil {
		logger.Log.WithError(err).Errorf("Error checking account age for account %s", account.Title)
		respondToInteraction(s, i, "There was an error checking the account age.")
		return
	}

	embed := createAccountAgeEmbed(account, years, months, days)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction with account age")
		respondToInteraction(s, i, "Error displaying account age. Please try again.")
	}

	// Log successful age check for admin monitoring
	services.IncrementSuccessfulAgeChecks()
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

func getAccounts(userID string) ([]models.Account, error) {
	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	return accounts, result.Error
}

func getAccount(accountID int) (models.Account, error) {
	var account models.Account
	result := database.DB.First(&account, accountID)
	return account, result.Error
}

func createAccountButtons(accounts []models.Account) []discordgo.MessageComponent {
	var components []discordgo.MessageComponent
	for _, account := range accounts {
		components = append(components, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("account_age_%d", account.ID),
		})
	}
	return components
}

func createAccountAgeEmbed(account models.Account, years, months, days int) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Account Age", account.Title),
		Description: fmt.Sprintf("The account is %d years, %d months, and %d days old.", years, months, days),
		Color:       0x00ff00,
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Last Status",
				Value:  string(account.LastStatus),
				Inline: true,
			},
			{
				Name:   "Creation Date",
				Value:  time.Now().AddDate(-years, -months, -days).Format("January 2, 2006"),
				Inline: true,
			},
		},
	}
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
