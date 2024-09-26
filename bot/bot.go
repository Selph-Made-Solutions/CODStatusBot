package bot

import (
	"errors"
	"os"
	"strings"

	"CODStatusBot/command"
	"CODStatusBot/command/accountlogs"
	"CODStatusBot/command/addaccount"
	"CODStatusBot/command/feedback"
	"CODStatusBot/command/removeaccount"
	"CODStatusBot/command/setcaptchaservice"
	"CODStatusBot/command/setcheckinterval"
	"CODStatusBot/command/togglecheck"
	"CODStatusBot/command/updateaccount"
	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
	"github.com/bwmarrin/discordgo"
)

var discord *discordgo.Session

func StartBot() (*discordgo.Session, error) {
	envToken := os.Getenv("DISCORD_TOKEN")
	if envToken == "" {
		return nil, errorhandler.NewValidationError(errors.New("DISCORD_TOKEN not set"), "Discord token")
	}

	var err error
	discord, err = discordgo.New("Bot " + envToken)
	if err != nil {
		return nil, errorhandler.NewAPIError(err, "Discord")
	}

	err = discord.Open()
	if err != nil {
		return nil, errorhandler.NewAPIError(err, "Discord")
	}

	err = discord.UpdateWatchStatus(0, "the Status of your Accounts so you dont have to.")
	if err != nil {
		return nil, errorhandler.NewDiscordError(err, "Discord")
	}

	command.RegisterCommands(discord)
	logger.Log.Info("Registering global commands")

	discord.AddHandler(
		func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			switch i.Type {
			case discordgo.InteractionApplicationCommand:
				command.HandleCommand(s, i)
			case discordgo.InteractionModalSubmit:
				handleModalSubmit(s, i)
			case discordgo.InteractionMessageComponent:
				handleMessageComponent(s, i)
			}
		}
	)

	go services.CheckAccounts(discord)
	return discord, nil
}
func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID
	var err error
	switch {
	case customID == "set_captcha_service_modal":
		err = setcaptchaservice.HandleModalSubmit(s, i)
	case customID == "add_account_modal":
		err = addaccount.HandleModalSubmit(s, i)
	case strings.HasPrefix(customID, "update_account_modal_"):
		err = updateaccount.HandleModalSubmit(s, i)
	case customID == "set_check_interval_modal":
		err = setcheckinterval.HandleModalSubmit(s, i)
	default:
		err = errorhandler.NewValidationError(fmt.Errorf("unknown modal submission: %s", customID), "modal submission")
	}

	if err != nil {
		userMsg, _ := errorhandler.HandleError(err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: userMsg,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	var err error
	switch {
	case strings.HasPrefix(customID, "feedback_"):
		err = feedback.HandleFeedbackChoice(s, i)
		//logger.Log.Info("Handling feedback choice")
	case strings.HasPrefix(customID, "account_age_"):
		err = accountage.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling account age selection")
	case strings.HasPrefix(customID, "account_logs_"):
		err = accountlogs.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling account logs selection")
	case customID == "account_logs_all":
		err = accountlogs.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling account logs selection")
	case strings.HasPrefix(customID, "update_account_"):
		err = updateaccount.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling update account selection")
	case strings.HasPrefix(customID, "remove_account_"):
		err = removeaccount.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling remove account selection")
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		err = removeaccount.HandleConfirmation(s, i)
		//logger.Log.Info("Handling remove account confirmation")
	case strings.HasPrefix(customID, "check_now_"):
		err = checknow.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling check now selection")
	case strings.HasPrefix(customID, "toggle_check_"):
		err = togglecheck.HandleAccountSelection(s, i)
		//logger.Log.Info("Handling toggle check selection")
	case strings.HasPrefix(customID, "confirm_reenable_") || customID == "cancel_reenable":
		err = togglecheck.HandleConfirmation(s, i)
		//logger.Log.Info("Handling toggle check confirmation")
	default:
		err = errorhandler.NewValidationError(fmt.Errorf("unknown message component interaction: %s", customID), "message component")
		//logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
	}

	if err != nil {
		userMsg, _ := errorhandler.HandleError(err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: userMsg,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}
