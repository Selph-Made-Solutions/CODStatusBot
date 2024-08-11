package services

import (
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

	expirationTimestamp, err := strconv.ParseInt(expirationStr, 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse expiration timestamp: %v", err)
	}

	expirationTime := time.Unix(expirationTimestamp, 0)

	return accountID, expirationTime, nil
}

func CheckSSOCookieExpiration(encodedStr string) (time.Duration, error) {
	_, expirationTime, err := DecodeSSOCookie(encodedStr)
	if err != nil {
		return 0, err
	}

	timeUntilExpiration := time.Until(expirationTime)
	return timeUntilExpiration, nil
}

func FormatExpirationTime(expirationTime time.Time) string {
	now := time.Now()
	duration := expirationTime.Sub(now)

	if duration <= 0 {
		return "Expired"
	}

	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d days, %d hours, %d minutes", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%d hours, %d minutes", hours, minutes)
	} else {
		return fmt.Sprintf("%d minutes", minutes)
	}
}
