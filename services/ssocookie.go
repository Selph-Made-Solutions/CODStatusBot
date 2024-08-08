package services

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

func DecodeSSOCookie(encodedStr string) (string, time.Time, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr)
	if err != nil {
		return "", time.Time{}, err
	}

	decodedStr := string(decodedBytes)
	parts := strings.Split(decodedStr, ":")

	if len(parts) < 3 {
		return "", time.Time{}, err
	}

	accountID := parts[0]
	expirationTimestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", time.Time{}, err
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
