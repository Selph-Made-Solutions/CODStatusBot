package errorhandler

import (
	"CODStatusBot/logger"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"strings"
)

type ErrorCategory int

const (
	NetworkError ErrorCategory = iota
	DatabaseError
	APIError
	ValidationError
	AuthenticationError
	RateLimitError
	DiscordError
	UnknownError
)

type CustomError struct {
	Category         ErrorCategory
	OriginalErr      error
	UserMessage      string
	AdminMessage     string
	IsUserActionable bool
}

var adminSession *discordgo.Session

func (e *CustomError) Error() string {
	return e.OriginalErr.Error()
}

func init() {
	InitAdminNotifications(os.Getenv("ADMIN_TOKEN"))
}

func InitAdminNotifications(token string) error {
	var err error
	adminSession, err = discordgo.New("Bot " + token)
	if err != nil {
		return err
	}
	return adminSession.Open()
}

func NotifyAdmin(message string) {
	adminID := os.Getenv("DEVELOPER_ID")
	if adminID == "" {
		return
	}

	channel, err := adminSession.UserChannelCreate(adminID)
	if err != nil {
		return
	}

	_, _ = adminSession.ChannelMessageSend(channel.ID, fmt.Sprintf("Admin Notification: %s", message))
}

func NewError(category ErrorCategory, err error, context string, userMsg string, isUserActionable bool) *CustomError {
	return &CustomError{
		Category:         category,
		OriginalErr:      err,
		UserMessage:      userMsg,
		AdminMessage:     fmt.Sprintf("%s: %v", context, err),
		IsUserActionable: isUserActionable,
	}
}

func HandleError(err error) (string, bool) {
	if customErr, ok := err.(*CustomError); ok {
		logger.Log.WithError(customErr.OriginalErr).
			WithField("category", customErr.Category).
			WithField("userActionable", customErr.IsUserActionable).
			Error(customErr.AdminMessage)

		if !customErr.IsUserActionable {
			NotifyAdmin(fmt.Sprintf("Critical error: %s", customErr.AdminMessage))
			return "An unexpected error occurred. Our team has been notified and is working on it.", false
		}

		return customErr.UserMessage, true
	}

	logger.Log.WithError(err).Error("Unexpected error occurred")
	NotifyAdmin(fmt.Sprintf("Unexpected error: %v", err))
	return "An unexpected error occurred. Our team has been notified and is working on it.", false
}

func NewNetworkError(err error, context string) *CustomError {
	return NewError(
		NetworkError,
		err,
		fmt.Sprintf("Network error: %s", context),
		"We're having trouble connecting to our servers. Please try again later.",
		false,
	)
}

func NewDatabaseError(err error, context string) *CustomError {
	return NewError(
		DatabaseError,
		err,
		fmt.Sprintf("Database error: %s", context),
		"We're experiencing database issues. Please try again later.",
		false,
	)
}

func NewAPIError(err error, api string) *CustomError {
	return NewError(
		APIError,
		err,
		fmt.Sprintf("%s API error", api),
		fmt.Sprintf("We're having trouble communicating with the %s API. Please try again later.", api),
		false,
	)
}

func NewValidationError(err error, field string) *CustomError {
	return NewError(
		ValidationError,
		err,
		fmt.Sprintf("Validation error: %s", field),
		fmt.Sprintf("The %s you provided is not valid. Please check and try again.", field),
		true,
	)
}

func NewAuthenticationError(err error) *CustomError {
	return NewError(
		AuthenticationError,
		err,
		"Authentication error",
		"Your session has expired or is invalid. Please log in again.",
		true,
	)
}

func NewRateLimitError(err error, limit string) *CustomError {
	return NewError(
		RateLimitError,
		err,
		fmt.Sprintf("Rate limit reached: %s", limit),
		fmt.Sprintf("You've reached the rate limit for this action. Please wait %s before trying again.", limit),
		true,
	)
}

func NewDiscordError(err error, context string) *CustomError {
	return NewError(
		DiscordError,
		err,
		fmt.Sprintf("Discord error: %s", context),
		"We're having trouble communicating with Discord. Please try again later.",
		false,
	)
}

func IsNetworkError(err error) bool {
	if customErr, ok := err.(*CustomError); ok {
		return customErr.Category == NetworkError
	}
	return strings.Contains(err.Error(), "network") || strings.Contains(err.Error(), "connection")
}

func IsDatabaseError(err error) bool {
	if customErr, ok := err.(*CustomError); ok {
		return customErr.Category == DatabaseError
	}
	return strings.Contains(err.Error(), "database") || strings.Contains(err.Error(), "sql")
}
