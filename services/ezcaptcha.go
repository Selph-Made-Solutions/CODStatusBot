package services

import (
	"CODStatusBot/logger"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	EZCaptchaAPIURL     = "https://api.ez-captcha.com/createTask"
	EZCaptchaResultURL  = "https://api.ez-captcha.com/getTaskResult"
	EZCaptchaBalanceURL = "https://api.ez-captcha.com/getBalance"
	MaxRetries          = 5
	RetryInterval       = 10 * time.Second
)

var (
	clientKey        string
	ezappID          string
	siteKey          string
	pageURL          string
	ezCaptchaLimiter *rate.Limiter
	ezCaptchaMu      sync.Mutex
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

	if clientKey == "" || siteKey == "" || pageURL == "" {
		return fmt.Errorf("EZCAPTCHA_CLIENT_KEY, RECAPTCHA_SITE_KEY, or RECAPTCHA_URL is not set in the environment")
	}
	return nil
}

func init() {
	// Allow 10 requests per second with a burst of 50
	ezCaptchaLimiter = rate.NewLimiter(rate.Every(100*time.Millisecond), 50)
}

func SolveReCaptchaV2(userID string) (string, error) {
	logger.Log.Info("Starting to solve ReCaptcha V2 using EZ-Captcha")
	ezCaptchaMu.Lock()
	err := ezCaptchaLimiter.Wait(context.Background())
	ezCaptchaMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("rate limit exceeded: %v", err)
	}

	apiKey, err := GetUserCaptchaKey(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get captcha key: %v", err)
	}

	logger.Log.Infof("Using API key: %s", apiKey)

	taskID, err := createTaskWithKey(apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create task: %v", err)
	}

	logger.Log.Infof("Task created with ID: %s", taskID)

	solution, err := getTaskResultWithKey(taskID, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to get task result: %v", err)
	}

	logger.Log.Info("Successfully solved ReCaptcha V2")
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
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	resp, err := http.Post(EZCaptchaAPIURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to send createTask request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	var result createTaskResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %v", err)
	}

	if result.ErrorID != 0 {
		return "", fmt.Errorf("EZ-Captcha error: %s (Error ID: %d, Error Code: %s)", result.ErrorDescription, result.ErrorID, result.ErrorCode)
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
			return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
		}

		resp, err := http.Post(EZCaptchaResultURL, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			return "", fmt.Errorf("failed to send getTaskResult request: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %v", err)
		}

		var result getTaskResultResponse
		err = json.Unmarshal(body, &result)
		if err != nil {
			return "", fmt.Errorf("failed to parse JSON response: %v", err)
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		if result.ErrorID != 0 {
			return "", fmt.Errorf("EZ-Captcha error: %s (Error ID: %d, Error Code: %s)", result.ErrorDescription, result.ErrorID, result.ErrorCode)
		}

		logger.Log.Infof("Task not ready, retrying in %v (Attempt %d/%d)", RetryInterval, i+1, MaxRetries)
		time.Sleep(RetryInterval)
	}

	return "", errors.New("max retries reached, captcha solving timed out")
}

func ValidateCaptchaKey(apiKey string) (bool, error) {
	payload := map[string]string{
		"clientKey": apiKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	resp, err := http.Post(EZCaptchaBalanceURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false, fmt.Errorf("failed to send getBalance request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %v", err)
	}

	var result struct {
		ErrorID int     `json:"errorId"`
		Balance float64 `json:"balance"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return false, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	if result.ErrorID != 0 {
		return false, nil
	}

	return true, nil
}
