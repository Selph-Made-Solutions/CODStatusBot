package services

import (
	"CODStatusBot/logger"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func DecodeSSOCookie(encodedStr string) (string, time.Time, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode base64: %v", err)
	}

	decodedStr := string(decodedBytes)
	parts := strings.Split(decodedStr, ":")

	if len(parts) < 3 {
		return "", time.Time{}, fmt.Errorf("invalid cookie format")
	}

	accountID := parts[0]
	expirationStr := parts[1]

	logger.Log.Infof("Decoded cookie parts - AccountID: %s, ExpirationStr: %s", accountID, expirationStr)

	expirationTimestamp, err := strconv.ParseInt(expirationStr, 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse expiration timestamp: %v", err)
	}

	// Assume the timestamp is in milliseconds
	expirationTime := time.UnixMilli(expirationTimestamp)

	logger.Log.Infof("Parsed expiration time: %v", expirationTime)

	return accountID, expirationTime, nil
}

func CheckSSOCookieExpiration(encodedStr string) (time.Duration, error) {
	_, expirationTime, err := DecodeSSOCookie(encodedStr)
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	timeUntilExpiration := expirationTime.Sub(now)

	logger.Log.Infof("Current time (UTC): %v", now)
	logger.Log.Infof("Expiration time (UTC): %v", expirationTime)
	logger.Log.Infof("Time until expiration: %v", timeUntilExpiration)

	if timeUntilExpiration <= 0 {
		return 0, nil // Cookie has expired
	}

	maxDuration := 14 * 24 * time.Hour // 14 days
	if timeUntilExpiration > maxDuration {
		return maxDuration, nil
	}

	return timeUntilExpiration, nil
}

func FormatExpirationTime(expirationTime time.Time) string {
	now := time.Now().UTC()
	timeUntilExpiration := expirationTime.Sub(now)

	logger.Log.Infof("Formatting expiration time - Current time (UTC): %v, Expiration time (UTC): %v, Time until expiration: %v", now, expirationTime, timeUntilExpiration)

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
