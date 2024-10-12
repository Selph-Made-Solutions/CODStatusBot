package services

import (
	"CODStatusBot/api2captcha"
	"CODStatusBot/logger"
	"errors"
	"os"
)

type CaptchaSolver interface {
	SolveReCaptchaV2(siteKey, pageURL string) (string, error)
}

type EZCaptchaSolver struct {
	APIKey string
}

type TwoCaptchaSolver struct {
	Client *api2captcha.Client
}

func NewCaptchaSolver(apiKey, provider string) (CaptchaSolver, error) {
	switch provider {
	case "ezcaptcha":
		return &EZCaptchaSolver{APIKey: apiKey}, nil
	case "2captcha":
		return &TwoCaptchaSolver{Client: api2captcha.NewClient(apiKey)}, nil
	default:
		return nil, errors.New("unsupported captcha provider")
	}
}

func (s *EZCaptchaSolver) SolveReCaptchaV2(siteKey, pageURL string) (string, error) {
	// Use the existing EZCaptcha solving logic
	return SolveReCaptchaV2WithKey(s.APIKey)
}

func (s *TwoCaptchaSolver) SolveReCaptchaV2(siteKey, pageURL string) (string, error) {
	recaptcha := api2captcha.ReCaptcha{
		SiteKey: siteKey,
		Url:     pageURL,
	}

	req := recaptcha.ToRequest()

	resp, id, err := s.Client.Solve(req)
	if err != nil {
		return "", err
	}

	logger.Log.Infof("2captcha solved captcha with ID: %s", id)
	return resp, nil
}

func GetCaptchaSolver(userID string) (CaptchaSolver, error) {
	userSettings, err := GetUserSettings(userID)
	if err != nil {
		return nil, err
	}

	var apiKey string
	var provider string

	if userSettings.PreferredCaptchaProvider == "2captcha" && userSettings.TwoCaptchaAPIKey != "" {
		apiKey = userSettings.TwoCaptchaAPIKey
		provider = "2captcha"
	} else if userSettings.EZCaptchaAPIKey != "" {
		apiKey = userSettings.EZCaptchaAPIKey
		provider = "ezcaptcha"
	} else {
		// Use default EZCaptcha key if user hasn't set a custom key
		apiKey = os.Getenv("EZCAPTCHA_CLIENT_KEY")
		provider = "ezcaptcha"
	}

	return NewCaptchaSolver(apiKey, provider)
}
