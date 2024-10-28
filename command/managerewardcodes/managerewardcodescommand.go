package managerewardcodes

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/utils"
	"github.com/bwmarrin/discordgo"
)

func CommandManageRewardCodes(s *discordgo.Session, i *discordgo.InteractionCreate) {
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		respondToInteraction(s, i, "Configuration error: Developer ID not set.")
		return
	}

	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	if userID != developerID {
		respondToInteraction(s, i, "You don't have permission to manage reward codes.")
		return
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Add New Code",
					Style:    discordgo.PrimaryButton,
					CustomID: "add_reward_code",
				},
				discordgo.Button{
					Label:    "List Active Codes",
					Style:    discordgo.SecondaryButton,
					CustomID: "list_reward_codes",
				},
				discordgo.Button{
					Label:    "Remove Code",
					Style:    discordgo.DangerButton,
					CustomID: "remove_reward_code",
				},
			},
		},
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an action to manage reward codes:",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error showing manage reward codes options")
	}
}

func HandleManageChoice(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	switch customID {
	case "add_reward_code":
		showAddCodeModal(s, i)
	case "list_reward_codes":
		listActiveCodes(s, i)
	case "remove_reward_code":
		showRemoveCodeOptions(s, i)
	}
}

func showAddCodeModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "add_reward_code_modal",
			Title:    "Add New Reward Code",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "code",
							Label:     "Code",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MinLength: 1,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "description",
							Label:     "Description",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MinLength: 1,
							MaxLength: 100,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "expires_in_days",
							Label:       "Expires in Days",
							Style:       discordgo.TextInputShort,
							Required:    true,
							Placeholder: "Enter number of days until expiration",
							MinLength:   1,
							MaxLength:   3,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "single_use",
							Label:     "Single Use (yes/no)",
							Style:     discordgo.TextInputShort,
							Required:  true,
							Value:     "no",
							MinLength: 2,
							MaxLength: 3,
						},
					},
				},
			},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error showing add code modal")
	}
}

func HandleAddCodeModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	var code, description, expiresInDays, singleUse string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok {
					switch textInput.CustomID {
					case "code":
						code = utils.SanitizeInput(strings.TrimSpace(textInput.Value))
					case "description":
						description = utils.SanitizeInput(strings.TrimSpace(textInput.Value))
					case "expires_in_days":
						expiresInDays = utils.SanitizeInput(strings.TrimSpace(textInput.Value))
					case "single_use":
						singleUse = strings.ToLower(utils.SanitizeInput(strings.TrimSpace(textInput.Value)))
					}
				}
			}
		}
	}

	// Validate inputs
	days, err := strconv.Atoi(expiresInDays)
	if err != nil || days <= 0 {
		respondToInteraction(s, i, "Please enter a valid number of days for expiration.")
		return
	}

	isSingleUse := singleUse == "yes"

	rewardCode := models.RewardCode{
		Code:        code,
		Description: description,
		ExpiresAt:   time.Now().AddDate(0, 0, days),
		SingleUse:   isSingleUse,
		Active:      true,
		UsedBy:      []string{},
	}

	if err := database.DB.Create(&rewardCode).Error; err != nil {
		logger.Log.WithError(err).Error("Error creating reward code")
		respondToInteraction(s, i, "Error adding reward code. Please try again.")
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Successfully added reward code:\nCode: %s\nDescription: %s\nExpires: %s\nSingle Use: %v",
		code, description, rewardCode.ExpiresAt.Format("2006-01-02"), isSingleUse))
}

func listActiveCodes(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var codes []models.RewardCode
	if err := database.DB.Where("active = ? AND expires_at > ?", true, time.Now()).Find(&codes).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching reward codes")
		respondToInteraction(s, i, "Error fetching reward codes. Please try again.")
		return
	}

	if len(codes) == 0 {
		respondToInteraction(s, i, "No active reward codes found.")
		return
	}

	var message strings.Builder
	message.WriteString("Active Reward Codes:\n\n")
	for _, code := range codes {
		message.WriteString(fmt.Sprintf("Code: %s\nDescription: %s\nExpires: %s\nSingle Use: %v\nTimes Used: %d\n\n",
			code.Code,
			code.Description,
			code.ExpiresAt.Format("2006-01-02"),
			code.SingleUse,
			len(code.UsedBy)))
	}

	respondToInteraction(s, i, message.String())
}

func showRemoveCodeOptions(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var codes []models.RewardCode
	if err := database.DB.Where("active = ? AND expires_at > ?", true, time.Now()).Find(&codes).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching reward codes")
		respondToInteraction(s, i, "Error fetching reward codes. Please try again.")
		return
	}

	if len(codes) == 0 {
		respondToInteraction(s, i, "No active reward codes to remove.")
		return
	}

	var components []discordgo.MessageComponent
	var currentRow []discordgo.MessageComponent

	for _, code := range codes {
		currentRow = append(currentRow, discordgo.Button{
			Label:    code.Code,
			Style:    discordgo.DangerButton,
			CustomID: fmt.Sprintf("remove_code_%s", code.Code),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select a code to remove:",
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error showing remove code options")
	}
}

func HandleRemoveCode(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	code := strings.TrimPrefix(customID, "remove_code_")

	if err := database.DB.Model(&models.RewardCode{}).Where("code = ?", code).Update("active", false).Error; err != nil {
		logger.Log.WithError(err).Error("Error removing reward code")
		respondToInteraction(s, i, "Error removing reward code. Please try again.")
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Successfully removed reward code: %s", code))
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
