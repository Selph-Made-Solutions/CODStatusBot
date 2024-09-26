package errorhandler

import (
	"fmt"
	"strings"

	"CODStatusBot/admin"
	"CODStatusBot/logger"
)

// ErrorCategory represents the category of an error

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

// CustomError represents a custom error with additional context

type CustomError struct {
	Category         ErrorCategory
	OriginalErr      error
	UserMessage      string
	AdminMessage     string
	IsUserActionable bool
}

// Error implements the error interface

func (e *CustomError) Error() string {
	return e.OriginalErr.Error()
}

// NewError creates a new CustomError

func NewError(category ErrorCategory, err error, userMsg, adminMsg string, isUserActionable bool) *CustomError {
	return &CustomError{
		Category:         category,
		OriginalErr:      err,
		UserMessage:      userMsg,
		AdminMessage:     adminMsg,
		IsUserActionable: isUserActionable,
	}
}

// HandleError processes the error and returns appropriate messages

func HandleError(err error) (string, bool) {
	if customErr, ok := err.(*CustomError); ok {
		logger.Log.WithError(customErr.OriginalErr).
			WithField("category", customErr.Category).
			WithField("userActionable", customErr.IsUserActionable).
			Error(customErr.AdminMessage)

		if !customErr.IsUserActionable {
			admin.NotifyAdmin(fmt.Sprintf("Critical error: %s", customErr.AdminMessage))
			return "An unexpected error occurred. Our team has been notified and is working on it.", false
		}

		return customErr.UserMessage, true
	}

	// For non-custom errors, log and notify admin
	logger.Log.WithError(err).Error("Unexpected error occurred")
	admin.NotifyAdmin(fmt.Sprintf("Unexpected error: %v", err))
	return "An unexpected error occurred. Our team has been notified and is working on it.", false
}

// Helper functions for common errors

func NewNetworkError(err error, details string) *CustomError {
	return NewError(
		NetworkError,
		err,
		"We're having trouble connecting to our servers. Please try again later.",
		fmt.Sprintf("Network error: %v. Details: %s", err, details),
		false,
	)
}

func NewDatabaseError(err error, operation string) *CustomError {
	return NewError(
		DatabaseError,
		err,
		"We're experiencing database issues. Please try again later.",
		fmt.Sprintf("Database error during %s: %v", operation, err),
		false,
	)
}

func NewAPIError(err error, api string) *CustomError {
	return NewError(
		APIError,
		err,
		fmt.Sprintf("We're having trouble communicating with the %s API. Please try again later.", api),
		fmt.Sprintf("%s API error: %v", api, err),
		false,
	)
}

func NewValidationError(err error, field string) *CustomError {
	return NewError(
		ValidationError,
		err,
		fmt.Sprintf("The %s you provided is not valid. Please check and try again.", field),
		fmt.Sprintf("Validation error for %s: %v", field, err),
		true,
	)
}

func NewAuthenticationError(err error) *CustomError {
	return NewError(
		AuthenticationError,
		err,
		"Your session has expired or is invalid. Please log in again.",
		fmt.Sprintf("Authentication error: %v", err),
		true,
	)
}

func NewRateLimitError(err error, limit string) *CustomError {
	return NewError(
		RateLimitError,
		err,
		fmt.Sprintf("You've reached the rate limit for this action. Please wait %s before trying again.", limit),
		fmt.Sprintf("Rate limit reached: %v", err),
		true,
	)
}

func NewDiscordError(err error, discordMsg string) *CustomError {
	return NewError(
		DiscordError,
		err,
		fmt.Sprintf("Unexpected Discord Error: Discord Response: %s", discordMsg),
		fmt.Sprintf("Discord error: %v", err),
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
