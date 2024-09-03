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
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

var Handlers = map[string]func(bot.Client, *events.ApplicationCommandInteractionCreate, models.InstallationType) error{}

func RegisterCommands(client bot.Client) {
	logger.Log.Info("Registering global commands")

	commands := []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:        "globalannouncement",
			Description: "Send a global announcement to all users (Admin only)",
		},
		discord.SlashCommandCreate{
			Name:        "setcaptchaservice",
			Description: "Set your EZ-Captcha API key",
		},
		discord.SlashCommandCreate{
			Name:        "setcheckinterval",
			Description: "Set check interval, notification interval, and notification type",
		},
		discord.SlashCommandCreate{
			Name:        "addaccount",
			Description: "Add a new account to monitor",
		},
		discord.SlashCommandCreate{
			Name:        "helpapi",
			Description: "Get help on using the bot and setting up your API key",
		},
		discord.SlashCommandCreate{
			Name:        "helpcookie",
			Description: "Simple guide to getting your SSOCookie",
		},
		discord.SlashCommandCreate{
			Name:        "accountage",
			Description: "Check the age of an account",
		},
		discord.SlashCommandCreate{
			Name:        "accountlogs",
			Description: "View the logs for an account",
		},
		discord.SlashCommandCreate{
			Name:        "checknow",
			Description: "Check account status now (rate limited for default API key)",
		},
		discord.SlashCommandCreate{
			Name:        "listaccounts",
			Description: "List all your monitored accounts",
		},
		discord.SlashCommandCreate{
			Name:        "removeaccount",
			Description: "Remove a monitored account",
		},
		discord.SlashCommandCreate{
			Name:        "updateaccount",
			Description: "Update a monitored account's information",
		},
		discord.SlashCommandCreate{
			Name:        "feedback",
			Description: "Send anonymous feedback to the bot developer",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "message",
					Description: "Your feedback or suggestion",
					Required:    true,
				},
			},
		},
		discord.SlashCommandCreate{
			Name:        "togglecheck",
			Description: "Toggle checks on/off for a monitored account",
		},
	}

	_, err := client.Rest().SetGlobalCommands(client.ApplicationID(), commands)
	if err != nil {
		logger.Log.WithError(err).Error("Error registering global commands")
		return
	}

	// Set up command handlers
	Handlers["globalannouncement"] = globalannouncement.CommandGlobalAnnouncement
	Handlers["setcaptchaservice"] = setcaptchaservice.CommandSetCaptchaService
	Handlers["setcheckinterval"] = setcheckinterval.CommandSetCheckInterval
	Handlers["addaccount"] = addaccount.CommandAddAccount
	Handlers["helpcookie"] = helpcookie.CommandHelpCookie
	Handlers["helpapi"] = helpapi.CommandHelpApi
	Handlers["feedback"] = feedback.CommandFeedback
	Handlers["accountage"] = accountage.CommandAccountAge
	Handlers["accountlogs"] = accountlogs.CommandAccountLogs
	Handlers["checknow"] = checknow.CommandCheckNow
	Handlers["listaccounts"] = listaccounts.CommandListAccounts
	Handlers["removeaccount"] = removeaccount.CommandRemoveAccount
	Handlers["updateaccount"] = updateaccount.CommandUpdateAccount
	Handlers["togglecheck"] = togglecheck.CommandToggleCheck

	logger.Log.Info("Global commands registered and handlers set up")
}

// HandleCommand handles incoming commands and checks for announcements
func HandleCommand(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) {
	// Check if the user has seen the announcement
	userID := event.User().ID.String()

	// Check if the user has seen the announcement
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
	} else if !userSettings.HasSeenAnnouncement {
		// Send the announcement to the user
		if err := globalannouncement.SendGlobalAnnouncement(client, userID); err != nil {
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
	if handler, ok := Handlers[event.Data.CommandName()]; ok {
		err := handler(client, event, installType)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error handling command: %s", event.Data.CommandName())
			event.CreateMessage(discord.MessageCreate{
				Content: "An error occurred while processing your request. Please try again later.",
				Flags:   discord.MessageFlagEphemeral,
			})
		}
	} else {
		logger.Log.Errorf("Unknown command: %s", event.Data.CommandName())
		event.CreateMessage(discord.MessageCreate{
			Content: "Unknown command. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}
}