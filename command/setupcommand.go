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
	"github.com/bwmarrin/discordgo"
)

var Handlers = map[string]func(*discordgo.Session, *discordgo.InteractionCreate, models.InstallationType){}

func RegisterCommands(s *discordgo.Session) {
	logger.Log.Info("Registering global commands")

	commands := []*discordgo.ApplicationCommand{
		{
			Name:             "globalannouncement",
			Description:      "Send a global announcement to all users (Admin only)",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(false),
		},
		{
			Name:             "setcaptchaservice",
			Description:      "Set your EZ-Captcha API key",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "setcheckinterval",
			Description:      "Set check interval, notification interval, and notification type",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "addaccount",
			Description:      "Add a new account to monitor",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "helpapi",
			Description:      "Get help on using the bot and setting up your API key",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "helpcookie",
			Description:      "Simple guide to getting your SSOCookie",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "accountage",
			Description:      "Check the age of an account",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "accountlogs",
			Description:      "View the logs for an account",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "checknow",
			Description:      "Check account status now (rate limited for default API key)",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "account_title",
					Description: "The title of the specific account to check (optional)",
					Required:    false,
				},
			},
		},
		{
			Name:             "listaccounts",
			Description:      "List all your monitored accounts",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "removeaccount",
			Description:      "Remove a monitored account",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "updateaccount",
			Description:      "Update a monitored account's information",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
		{
			Name:             "feedback",
			Description:      "Send anonymous feedback to the bot developer",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
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
			Name:             "togglecheck",
			Description:      "Toggle checks on/off for a monitored account",
			IntegrationTypes: []discordgo.IntegrationType{discordgo.UserInstallation, discordgo.GuildInstallation},
			ContextTypes:     []discordgo.InteractionContextType{discordgo.GuildInteraction, discordgo.BotDMInteraction, discordgo.PrivateChannelInteraction},
			DMPermission:     BoolPtr(true),
		},
	}

	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands)
	if err != nil {
		logger.Log.WithError(err).Error("Error registering global commands")
		return
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
}

// HandleCommand handles incoming commands and checks for announcements
func HandleCommand(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	// Check if the user has seen the announcement
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
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
	} else if !userSettings.HasSeenAnnouncement {
		// Send the announcement to the user
		if err := globalannouncement.SendGlobalAnnouncement(s, userID); err != nil {
			logger.Log.WithError(err).Error("Error sending announcement to user")
		} else {
			// Update the user's settings to mark the announcement as seen
			userSettings.HasSeenAnnouncement = true
			if err := database.DB.Save(&userSettings).Error; err != nil {
				logger.Log.WithError(err).Error("Error updating user settings after sending announcement")
			}
		}
	}

	// Continue with regular command handling
	if h, ok := Handlers[i.ApplicationCommandData().Name]; ok {
		h(s, i, installType)
	}
}

// BoolPtr Helper function to create a pointer to a bool
func BoolPtr(b bool) *bool {
	return &b
}
