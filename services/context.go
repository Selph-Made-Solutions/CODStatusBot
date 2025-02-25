package services

import (
	"fmt"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

type InstallContext string

const (
	ServerContext InstallContext = "server"
	DirectContext InstallContext = "direct"
)

func GetInstallContext(i *discordgo.InteractionCreate) InstallContext {
	if i.GuildID != "" {
		return ServerContext
	}
	return DirectContext
}

func GetUserID(i *discordgo.InteractionCreate) (string, error) {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID, nil
	}
	if i.User != nil {
		return i.User.ID, nil
	}
	return "", fmt.Errorf("unable to determine user ID")
}

func GetResponseChannel(s *discordgo.Session, userID string, i *discordgo.InteractionCreate) (string, error) {
	var userSettings models.UserSettings
	result := database.DB.Where("user_id = ?", userID).First(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user settings for channel determination")
	}

	context := GetInstallContext(i)

	if context == DirectContext {
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %w", err)
		}
		return channel.ID, nil
	}

	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %w", err)
		}
		return channel.ID, nil
	}

	if i.ChannelID != "" {
		return i.ChannelID, nil
	}

	var account models.Account
	if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err == nil && account.ChannelID != "" {
		return account.ChannelID, nil
	}

	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return "", fmt.Errorf("failed to create fallback DM channel: %w", err)
	}
	return channel.ID, nil
}

func SendMessageToUser(s *discordgo.Session, userID string, i *discordgo.InteractionCreate, message string) error {
	channelID, err := GetResponseChannel(s, userID, i)
	if err != nil {
		return fmt.Errorf("failed to determine response channel: %w", err)
	}

	_, err = s.ChannelMessageSend(channelID, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func SendEmbedToUser(s *discordgo.Session, userID string, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error {
	channelID, err := GetResponseChannel(s, userID, i)
	if err != nil {
		return fmt.Errorf("failed to determine response channel: %w", err)
	}

	_, err = s.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return fmt.Errorf("failed to send embed: %w", err)
	}

	return nil
}
