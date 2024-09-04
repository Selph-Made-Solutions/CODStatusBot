package services

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ... (previous code remains unchanged)

func CheckAccount(ssoCookie, userID string) (models.Status, error) {
	logger.Log.Info("Starting CheckAccount function")

	captchaAPIKey, err := GetUserCaptchaKey(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user's captcha API key")
		return models.StatusUnknown, fmt.Errorf("failed to get user's captcha API key: %v", err)
	}

	gRecaptchaResponse, err := SolveReCaptchaV2WithKey(captchaAPIKey)
	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to solve reCAPTCHA: %v", err)
	}

	logger.Log.Info("Successfully solved reCAPTCHA")

	banAppealUrl := fmt.Sprintf("%s&g-cc=%s", url1, gRecaptchaResponse)
	logger.Log.WithField("url", banAppealUrl).Info("Constructed ban appeal URL")

	var body []byte

	for attempt := 0; attempt < maxRetries; attempt++ {
		_, body, err = makeRequest("GET", banAppealUrl, ssoCookie)
		if err == nil {
			break
		}
		logger.Log.WithError(err).Errorf("Attempt %d failed to check account status", attempt+1)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to check account status after %d attempts: %v", maxRetries, err)
	}

	return parseAccountStatus(body)
}

// ... (rest of the file remains unchanged)
