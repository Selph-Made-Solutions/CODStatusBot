package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"CODStatusBot/logger"
)

const (
	EZCaptchaAPIURL     = "https://api.ez-captcha.com/createTask"
	EZCaptchaResultURL  = "https://api.ez-captcha.com/getTaskResult"
	EZCaptchaBalanceURL = "https://api.ez-captcha.com/getBalance"
	MaxRetries          = 6
	RetryInterval       = 10 * time.Second
)

var (
	clientKey  string
	ezappID    string
	siteKey    string
	pageURL    string
	siteAction string
)

type createTaskRequest struct {
	ClientKey string `json:"clientKey"`
	EzAppID   string `json:"appId"`
	Task      task   `json:"task"`
}

type task struct {
	Type        string `json:"type"`
	WebsiteURL  string `json:"websiteURL"`
	WebsiteKey  string `json:"websiteKey"`
	IsInvisible bool   `json:"isInvisible"`
	SiteAction  string `json:"siteAction,omitempty"`
}

type createTaskResponse struct {
	ErrorID          int    `json:"errorId"`
	ErrorCode        string `json:"errorCode"`
	ErrorDescription string `json:"errorDescription"`
	TaskID           string `json:"taskId"`
}

type getTaskResultRequest struct {
	ClientKey string `json:"clientKey"`
	TaskID    string `json:"taskId"`
}

type getTaskResultResponse struct {
	ErrorID          int    `json:"errorId"`
	ErrorCode        string `json:"errorCode"`
	ErrorDescription string `json:"errorDescription"`
	Status           string `json:"status"`
	Solution         struct {
		GRecaptchaResponse string `json:"gRecaptchaResponse"`
	} `json:"solution"`
}

func LoadEnvironmentVariables() error {
	clientKey = os.Getenv("EZCAPTCHA_CLIENT_KEY")
	ezappID = os.Getenv("EZAPPID")
	siteKey = os.Getenv("RECAPTCHA_SITE_KEY")
	pageURL = os.Getenv("RECAPTCHA_URL")
	siteaction := os.Getenv("SITE_ACTION")

	if clientKey == "" || siteKey == "" || pageURL == "" || ezappID == "" || siteaction == "" {
		return fmt.Errorf("EZCAPTCHA_CLIENT_KEY, RECAPTCHA_SITE_KEY, RECAPTCHA_URL, EZAPPID, or SITE_ACTION is not set in the environment")
	}
	return nil
}

func SolveReCaptchaV2WithKey(apiKey string) (string, error) {
	isValid, balance, err := ValidateCaptchaKey(apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to validate captcha key: %w", err)
	}

	if !isValid {
		return "", errors.New("invalid captcha API key")
	}

	if balance <= 0 {
		return "", errors.New("insufficient balance for captcha solving")
	}

	logger.Log.Infof("Captcha key validated. Balance: %.2f", balance)

	taskID, err := createTaskWithKey(apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create captcha task: %w", err)
	}

	logger.Log.Infof("Captcha task created. Task ID: %s", taskID)

	solution, err := getTaskResultWithKey(taskID, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to get captcha task result: %w", err)
	}

	logger.Log.Info("Captcha solved successfully")
	return solution, nil
}

func createTaskWithKey(apiKey string) (string, error) {
	payload := createTaskRequest{
		ClientKey: apiKey,
		EzAppID:   ezappID,
		Task: task{
			Type:        "ReCaptchaV2TaskProxyless",
			WebsiteURL:  pageURL,
			WebsiteKey:  siteKey,
			IsInvisible: false,
			SiteAction:  siteAction,
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	resp, err := http.Post(EZCaptchaAPIURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to send createTask request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result createTaskResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if result.ErrorID != 0 {
		return "", fmt.Errorf("EZ-Captcha error: %s", result.ErrorDescription)
	}

	return result.TaskID, nil
}

func getTaskResultWithKey(taskID string, apiKey string) (string, error) {
	for i := 0; i < MaxRetries; i++ {
		payload := getTaskResultRequest{
			ClientKey: apiKey,
			TaskID:    taskID,
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to marshal JSON payload: %w", err)
		}

		resp, err := http.Post(EZCaptchaResultURL, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			return "", fmt.Errorf("failed to send getTaskResult request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}

		var result getTaskResultResponse
		err = json.Unmarshal(body, &result)
		if err != nil {
			return "", fmt.Errorf("failed to parse JSON response: %w", err)
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		if result.ErrorID != 0 {
			return "", fmt.Errorf("EZ-Captcha error: %s", result.ErrorDescription)
		}

		logger.Log.Infof("Waiting for captcha solution. Attempt %d/%d", i+1, MaxRetries)
		time.Sleep(RetryInterval)
	}

	return "", errors.New("max retries reached, captcha solving timed out")
}

func ValidateCaptchaKey(apiKey string) (bool, float64, error) {
	payload := map[string]string{
		"clientKey": apiKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return false, 0, fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	resp, err := http.Post(EZCaptchaBalanceURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false, 0, fmt.Errorf("failed to send getBalance request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read response body: %v", err)
	}

	var result struct {
		ErrorId int     `json:"errorId"`
		Balance float64 `json:"balance"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	if result.ErrorId != 0 {
		return false, 0, nil
	}

	return true, result.Balance, nil
}
