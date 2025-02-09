package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/logger"
)

func DecodeSSOCookie(encodedStr string) (int64, error) {
	encodedStr = strings.TrimSpace(encodedStr)
	padding := len(encodedStr) % 4
	if padding != 0 {
		encodedStr += strings.Repeat("=", 4-padding)
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr)
	if err != nil {
		decodedBytes, err = base64.URLEncoding.DecodeString(encodedStr)
		if err != nil {
			return 0, fmt.Errorf("invalid SSO cookie format: %w", err)
		}
	}
	decodedStr := string(decodedBytes)
	parts := strings.Split(decodedStr, ":")
	if len(parts) < 2 {
		return 0, errors.New("invalid SSO cookie structure: missing timestamp")
	}
	expirationStr := parts[1]
	logger.Log.Infof("Decoded cookie expiration: %s", expirationStr)
	expirationTimestamp, err := strconv.ParseInt(expirationStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid expiration timestamp in SSO cookie: %w", err)
	}
	if len(expirationStr) > 10 {
		expirationTimestamp /= 1000
	}
	if expirationTimestamp < time.Now().Unix() {
		return 0, errors.New("SSO cookie has expired")
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
		return 0, fmt.Errorf("cookie has expired")
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
