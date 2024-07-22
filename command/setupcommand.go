package command

import (
	"CODStatusBot/command/accountage"
	"CODStatusBot/command/accountlogs"
	"CODStatusBot/command/addaccount"
	"CODStatusBot/command/checknow"
	"CODStatusBot/command/feedback"
	"CODStatusBot/command/help"
	"CODStatusBot/command/listaccounts"
	"CODStatusBot/command/removeaccount"
	"CODStatusBot/command/setpreference"
	"CODStatusBot/command/updateaccount"
	"CODStatusBot/logger"

	"github.com/bwmarrin/discordgo"
)

var Handlers = map[string]func(*discordgo.Session, *discordgo.InteractionCreate){}

func RegisterCommands(s *discordgo.Session) {
	logger.Log.Info("Registering global commands")

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "setpreference",
			Description: "Set your preference for where you want to receive status notifications",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "type",
					Description: "Where do you want to receive Status Notifications?",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Channel", Value: "channel"},
						{Name: "Direct Message", Value: "dm"},
					},
				},
			},
		},
		{
			Name:        "addaccount",
			Description: "Add a new account to monitor using a modal",
		},
		{
			Name:        "help",
			Description: "Simple guide to getting your SSOCookie",
		},
		{
			Name:        "accountage",
			Description: "Check the age of an account",
		},
		{
			Name:        "accountlogs",
			Description: "View the logs for an account",
		},
		{
			Name:        "checknow",
			Description: "Immediately check the status of all your accounts or a specific account",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "account_title",
					Description: "The title of the account to check (leave empty to check all accounts)",
					Required:    false,
				},
			},
		},
		{
			Name:        "listaccounts",
			Description: "List all your monitored accounts",
		},
		{
			Name:        "removeaccount",
			Description: "Remove a monitored account",
		},
		{
			Name:        "updateaccount",
			Description: "Update a monitored account's information",
		},
		{
			Name:        "feedback",
			Description: "Send anonymous feedback or suggestions to the bot developer",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "Your feedback or suggestion",
					Required:    true,
				},
			},
		},
	}

	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands)
	if err != nil {
		logger.Log.WithError(err).Error("Error registering global commands")
		return
	}

	// Set up command handlers
	Handlers["setpreference"] = setpreference.CommandSetPreference
	Handlers["addaccount"] = addaccount.CommandAddAccount
	Handlers["help"] = help.CommandHelp
	Handlers["accountage"] = accountage.CommandAccountAge
	Handlers["accountlogs"] = accountlogs.CommandAccountLogs
	Handlers["checknow"] = checknow.CommandCheckNow
	Handlers["listaccounts"] = listaccounts.CommandListAccounts
	Handlers["removeaccount"] = removeaccount.CommandRemoveAccount
	Handlers["updateaccount"] = updateaccount.CommandUpdateAccount
	Handlers["feedback"] = feedback.CommandFeedback

	logger.Log.Info("Global commands registered and handlers set up")
}
