package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/logger"
)

const (
	MaxRetries    = 6
	RetryInterval = 10 * time.Second

	EZCaptchaCreateEndpoint  = "https://api.ez-captcha.com/createTask"
	EZCaptchaResultEndpoint  = "https://api.ez-captcha.com/getTaskResult"
	TwoCaptchaCreateEndpoint = "https://api.2captcha.com/createTask"
	TwoCaptchaResultEndpoint = "https://api.2captcha.com/getTaskResult"
	//	twocap                   = "2captcha"
	//	ezcap                    = "ezcaptcha"
)

type CaptchaSolver interface {
	SolveReCaptchaV2(siteKey, pageURL string) (string, error)
}

type EZCaptchaSolver struct {
	APIKey  string
	EzappID string
}

type TwoCaptchaSolver struct {
	APIKey string
	SoftID string
}

func IsServiceEnabled(provider string) bool {
	cfg := configuration.Get()
	switch provider {
	case "ezcaptcha":
		return cfg.CaptchaService.EZCaptcha.Enabled
	case "2captcha":
		return cfg.CaptchaService.TwoCaptcha.Enabled
	default:
		return false
	}
}

func VerifyEZCaptchaConfig() bool {
	cfg := configuration.Get()
	ezConfig := cfg.CaptchaService

	if ezConfig.EZCaptcha.ClientKey == "" || ezConfig.EZCaptcha.AppID == "" || ezConfig.SiteAction == "" {
		logger.Log.Error("Missing required EZCaptcha configuration")
		return false
	}

	if ezConfig.RecaptchaSiteKey == "" {
		logger.Log.Error("RECAPTCHA_SITE_KEY is not set")
		return false
	}
	if ezConfig.RecaptchaURL == "" {
		logger.Log.Error("RECAPTCHA_URL is not set")
		return false
	}

	logger.Log.Info("EZCaptcha configuration verified successfully")
	return true
}

func NewCaptchaSolver(apiKey, provider string) (CaptchaSolver, error) {
	cfg := configuration.Get()

	if !IsServiceEnabled(provider) {
		return nil, fmt.Errorf("captcha service %s is currently disabled", provider)
	}

	switch provider {
	case "ezcaptcha":
		return &EZCaptchaSolver{
			APIKey:  apiKey,
			EzappID: cfg.CaptchaService.EZCaptcha.AppID,
		}, nil
	case "2captcha":
		return &TwoCaptchaSolver{
			APIKey: apiKey,
			SoftID: cfg.CaptchaService.TwoCaptcha.SoftID,
		}, nil
	default:
		return nil, errors.New("unsupported captcha provider")
	}
}

func (s *EZCaptchaSolver) SolveReCaptchaV2(siteKey, pageURL string) (string, error) {

	taskID, err := s.createTask(siteKey, pageURL)
	if err != nil {
		return "", fmt.Errorf("failed to create captcha task: %w", err)
	}
	return s.getTaskResult(taskID)
}

func (s *TwoCaptchaSolver) SolveReCaptchaV2(siteKey, pageURL string) (string, error) {
	taskID, err := s.createTask(siteKey, pageURL)
	if err != nil {
		return "", fmt.Errorf("failed to create captcha task: %w", err)
	}
	return s.getTaskResult(taskID)
}

func (s *EZCaptchaSolver) createTask(siteKey, pageURL string) (string, error) {
	cfg := configuration.Get()

	if s.EzappID == "" {
		return "", fmt.Errorf("EzappID is not configured")
	}

	if cfg.CaptchaService.SiteAction == "" {
		return "", fmt.Errorf("site action is not configured")
	}

	payload := map[string]interface{}{
		"clientKey": s.APIKey,
		"appId":     s.EzappID,
		"task": map[string]interface{}{
			"type":        "ReCaptchaV2TaskProxyless",
			"websiteURL":  pageURL,
			"websiteKey":  siteKey,
			"isInvisible": false,
			"sa":          cfg.CaptchaService.SiteAction,
		},
	}

	resp, err := sendRequest(EZCaptchaCreateEndpoint, payload)
	if err != nil {
		return "", err
	}

	var result struct {
		ErrorId          int    `json:"errorId"`
		ErrorCode        string `json:"errorCode"`
		ErrorDescription string `json:"errorDescription"`
		TaskId           string `json:"taskId"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.ErrorId != 0 {
		return "", fmt.Errorf("API error: %s - %s", result.ErrorCode, result.ErrorDescription)
	}

	return result.TaskId, nil
}

func (s *TwoCaptchaSolver) createTask(siteKey, pageURL string) (string, error) {
	payload := map[string]interface{}{
		"clientKey": s.APIKey,
		"softId":    s.SoftID,
		"task": map[string]interface{}{
			"type":       "RecaptchaV2TaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
		},
	}

	resp, err := sendRequest(TwoCaptchaCreateEndpoint, payload)
	if err != nil {
		return "", err
	}

	logger.Log.Infof("2captcha response: %s", string(resp))

	var result struct {
		ErrorId          int    `json:"errorId"`
		ErrorCode        string `json:"errorCode"`
		ErrorDescription string `json:"errorDescription"`
		TaskId           int64  `json:"taskId"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.ErrorId != 0 {
		return "", fmt.Errorf("API error creating task: %s - %s", result.ErrorCode, result.ErrorDescription)
	}

	return fmt.Sprintf("%d", result.TaskId), nil
}

func (s *EZCaptchaSolver) getTaskResult(taskID string) (string, error) {
	for i := 0; i < MaxRetries; i++ {
		payload := map[string]interface{}{
			"clientKey": s.APIKey,
			"taskId":    taskID,
		}

		resp, err := sendRequest(EZCaptchaResultEndpoint, payload)
		if err != nil {
			return "", err
		}

		var result struct {
			ErrorId          int    `json:"errorId"`
			ErrorCode        string `json:"errorCode"`
			ErrorDescription string `json:"errorDescription"`
			Status           string `json:"status"`
			Solution         struct {
				GRecaptchaResponse string `json:"gRecaptchaResponse"`
			} `json:"solution"`
		}

		if err := json.Unmarshal(resp, &result); err != nil {
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		if result.ErrorId != 0 {
			return "", fmt.Errorf("API error: %s - %s", result.ErrorCode, result.ErrorDescription)
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		time.Sleep(RetryInterval)
	}

	return "", errors.New("max retries reached waiting for result")
}

func (s *TwoCaptchaSolver) getTaskResult(taskID string) (string, error) {
	for i := 0; i < MaxRetries; i++ {
		payload := map[string]interface{}{
			"clientKey": s.APIKey,
			"taskId":    taskID,
		}

		resp, err := sendRequest(TwoCaptchaResultEndpoint, payload)
		if err != nil {
			return "", err
		}

		var result struct {
			ErrorId  int    `json:"errorId"`
			Status   string `json:"status"`
			Solution struct {
				GRecaptchaResponse string `json:"gRecaptchaResponse"`
			} `json:"solution"`
		}

		if err := json.Unmarshal(resp, &result); err != nil {
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		if result.ErrorId != 0 {
			return "", fmt.Errorf("API error getting result")
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		time.Sleep(RetryInterval)
	}

	return "", errors.New("max retries reached waiting for result")
}

func sendRequest(url string, payload interface{}) ([]byte, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println(err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

func getBalanceThreshold(provider string) float64 {
	cfg := configuration.Get()
	switch provider {
	case "ezcaptcha":
		return cfg.CaptchaService.EZCaptcha.BalanceMin
	case "2captcha":
		return cfg.CaptchaService.TwoCaptcha.BalanceMin
	default:
		return 0
	}
}

func ValidateCaptchaKey(apiKey, provider string) (bool, float64, error) {
	switch provider {
	case "ezcaptcha":
		return validateEZCaptchaKey(apiKey)
	case "2captcha":
		return validate2CaptchaKey(apiKey)
	default:
		return false, 0, errors.New("unsupported captcha provider")
	}
}

func validateEZCaptchaKey(apiKey string) (bool, float64, error) {
	url := "https://api.ez-captcha.com/getBalance"
	payload := map[string]string{
		"clientKey": apiKey,
		"action":    "getBalance",
	}

	resp, err := sendRequest(url, payload)
	if err != nil {
		return false, 0, err
	}

	var result struct {
		ErrorId int     `json:"errorId"`
		Balance float64 `json:"balance"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return false, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.ErrorId != 0 {
		return false, 0, nil
	}

	return true, result.Balance, nil
}

func validate2CaptchaKey(apiKey string) (bool, float64, error) {
	url := "https://api.2captcha.com/getBalance"
	payload := map[string]string{
		"clientKey": apiKey,
		"action":    "getBalance",
	}

	resp, err := sendRequest(url, payload)
	if err != nil {
		return false, 0, err
	}

	var result struct {
		ErrorId int     `json:"errorId"`
		Balance float64 `json:"balance"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return false, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.ErrorId != 0 {
		return false, 0, nil
	}

	return true, result.Balance, nil
}
