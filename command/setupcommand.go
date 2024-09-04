package command

import (
	"CODStatusBot/command/accountage"
	"CODStatusBot/command/accountlogs"
	"CODStatusBot/command/addaccount"
	"CODStatusBot/command/checknow"
	"CODStatusBot/command/feedback"
	"CODStatusBot/command/globalannouncement"
	"CODStatusBot/command/helpapi"
	"CODStatusBot/command/helpcookie"
	"CODStatusBot/command/listaccounts"
	"CODStatusBot/command/removeaccount"
	"CODStatusBot/command/setcaptchaservice"
	"CODStatusBot/command/setcheckinterval"
	"CODStatusBot/command/togglecheck"
	"CODStatusBot/command/updateaccount"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
)

// BoolPtr Helper function to create a pointer to a bool
func BoolPtr(b bool) *bool {
	return &b
}

var Handlers = map[string]func(*discordgo.Session, *discordgo.InteractionCreate, models.InstallationType){}

func RegisterCommands(s *discordgo.Session) error {
	logger.Log.Info("Registering global commands")

	commands := []*discordgo.ApplicationCommand{
		{
			Name:         "globalannouncement",
			Description:  "Send a global announcement to all users (Admin only)",
			DMPermission: BoolPtr(false),
		},
		{
			Name:         "setcaptchaservice",
			Description:  "Set your EZ-Captcha API key",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "setcheckinterval",
			Description:  "Set check interval, notification interval, and notification type",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "addaccount",
			Description:  "Add a new account to monitor",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "helpapi",
			Description:  "Get help on using the bot and setting up your API key",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "helpcookie",
			Description:  "Simple guide to getting your SSOCookie",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "accountage",
			Description:  "Check the age of an account",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "accountlogs",
			Description:  "View the logs for an account",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "checknow",
			Description:  "Check account status now (rate limited for default API key)",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "listaccounts",
			Description:  "List all your monitored accounts",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "removeaccount",
			Description:  "Remove a monitored account",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "updateaccount",
			Description:  "Update a monitored account's information",
			DMPermission: BoolPtr(true),
		},
		{
			Name:         "feedback",
			Description:  "Send anonymous feedback to the bot developer",
			DMPermission: BoolPtr(true),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "Your feedback or suggestion",
					Required:    true,
				},
			},
		},
		{
			Name:         "togglecheck",
			Description:  "Toggle checks on/off for a monitored account",
			DMPermission: BoolPtr(true),
		},
	}

	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands)
	if err != nil {
		logger.Log.WithError(err).Error("Error registering global commands")
		return err // Or return an error if something goes wrong
	}

	// Set up command handlers
	Handlers["globalannouncement"] = globalannouncement.CommandGlobalAnnouncement
	Handlers["setcaptchaservice"] = setcaptchaservice.CommandSetCaptchaService
	Handlers["setcheckinterval"] = setcheckinterval.CommandSetCheckInterval
	Handlers["addaccount"] = addaccount.CommandAddAccount
	Handlers["add_account_modal"] = addaccount.HandleModalSubmit
	Handlers["helpcookie"] = helpcookie.CommandHelpCookie
	Handlers["helpapi"] = helpapi.CommandHelpApi
	Handlers["feedback"] = feedback.CommandFeedback
	Handlers["accountage"] = accountage.CommandAccountAge
	Handlers["accountlogs"] = accountlogs.CommandAccountLogs
	Handlers["checknow"] = checknow.CommandCheckNow
	Handlers["listaccounts"] = listaccounts.CommandListAccounts
	Handlers["removeaccount"] = removeaccount.CommandRemoveAccount
	Handlers["remove_account_select"] = removeaccount.HandleAccountSelection
	Handlers["updateaccount"] = updateaccount.CommandUpdateAccount
	Handlers["update_account_modal"] = updateaccount.HandleModalSubmit
	Handlers["set_check_interval_modal"] = setcheckinterval.HandleModalSubmit
	Handlers["togglecheck"] = togglecheck.CommandToggleCheck

	logger.Log.Info("Global commands registered and handlers set up")
	return nil
}

// HandleCommand handles incoming commands and checks for announcements
func HandleCommand(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	logger.Log.Infof("Received command: %s", i.ApplicationCommandData().Name)

	if h, ok := Handlers[i.ApplicationCommandData().Name]; ok {
		logger.Log.Infof("Executing handler for command: %s", i.ApplicationCommandData().Name)

		defer func() {
			if r := recover(); r != nil {
				logger.Log.Errorf("Panic recovered in command handler for %s: %v", i.ApplicationCommandData().Name, r)
				sendErrorResponse(s, i, "An unexpected error occurred. Please try again later.")
			}
		}()

		h(s, i, installType)
		logger.Log.Infof("Finished executing handler for command: %s", i.ApplicationCommandData().Name)
	} else {
		logger.Log.Warnf("Unknown command: %s", i.ApplicationCommandData().Name)
		sendErrorResponse(s, i, fmt.Sprintf("Unknown command: %s", i.ApplicationCommandData().Name))
	}
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		return
	}
	// Check if the user has seen the announcement
	var userSettings models.UserSettings
	result := database.GetDB().Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
	} else if !userSettings.HasSeenAnnouncement {
		// Send the announcement to the user
	}
}
func sendErrorResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send error response")

	}
}
