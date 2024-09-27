package services

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
)

func DecodeSSOCookie(encodedStr string) (int64, error) {
	// Remove any potential whitespace or newline characters
	encodedStr = strings.TrimSpace(encodedStr)

	// Add padding if necessary
	if len(encodedStr)%4 != 0 {
		encodedStr += strings.Repeat("=", 4-len(encodedStr)%4)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr)
	if err != nil {
		return 0, errorhandler.NewValidationError(err, "decoding base64 SSO cookie")
	}

	decodedStr := string(decodedBytes)
	parts := strings.Split(decodedStr, ":")

	if len(parts) < 2 {
		return 0, errorhandler.NewValidationError(fmt.Errorf("invalid cookie format"), "SSO cookie structure")
	}

	expirationStr := parts[1]

	logger.Log.Infof("Decoded cookie expiration: %s", expirationStr)

	expirationTimestamp, err := strconv.ParseInt(expirationStr, 10, 64)
	if err != nil {
		return 0, errorhandler.NewValidationError(err, "parsing expiration timestamp")
	}

	// Convert milliseconds to seconds if necessary
	if len(expirationStr) > 10 {
		expirationTimestamp /= 1000
	}

	// Check if the timestamp is in the past
	if expirationTimestamp < time.Now().Unix() {
		return 0, errorhandler.NewValidationError(fmt.Errorf("SSO cookie has already expired"), "SSO cookie expiration")
	}

	return expirationTimestamp, nil
}

func CheckSSOCookieExpiration(expirationTimestamp int64) (time.Duration, error) {
	now := time.Now().Unix()
	timeUntilExpiration := time.Duration(expirationTimestamp-now) * time.Second

	logger.Log.Infof("Current time (Unix): %v", now)
	logger.Log.Infof("Expiration time (Unix): %v", expirationTimestamp)
	logger.Log.Infof("Time until expiration: %v", timeUntilExpiration)

	if timeUntilExpiration <= 0 {
		return 0, errorhandler.NewValidationError(fmt.Errorf("cookie has expired"), "SSO cookie expiration")
	}

	maxDuration := 14 * 24 * time.Hour // 14 days
	if timeUntilExpiration > maxDuration {
		return maxDuration, nil
	}

	return timeUntilExpiration, nil
}

func FormatExpirationTime(expirationTimestamp int64) string {
	timeUntilExpiration := time.Duration(expirationTimestamp-time.Now().Unix()) * time.Second

	logger.Log.Infof("Formatting expiration time - Current time (Unix): %v, Expiration time (Unix): %v, Time until expiration: %v", time.Now().Unix(), expirationTimestamp, timeUntilExpiration)

	if timeUntilExpiration <= 0 {
		return "Expired"
	}

	maxDuration := 14 * 24 * time.Hour // 14 days
	if timeUntilExpiration > maxDuration {
		timeUntilExpiration = maxDuration
	}

	days := int(timeUntilExpiration.Hours() / 24)
	hours := int(timeUntilExpiration.Hours()) % 24

	if days > 0 {
		return fmt.Sprintf("%d days, %d hours", days, hours)
	} else {
		return fmt.Sprintf("%d hours", hours)
	}
}
