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

type CaptchaSolver interface {
	SolveReCaptchaV2(siteKey, pageURL string) (string, error)
}

type EZCaptchaSolver struct {
	APIKey string
}

type TwoCaptchaSolver struct {
	APIKey string
	SoftID int
}

const (
	MaxRetries    = 6
	RetryInterval = 10 * time.Second
)

func NewCaptchaSolver(apiKey, provider string) (CaptchaSolver, error) {
	switch provider {
	case "ezcaptcha":

		return &EZCaptchaSolver{APIKey: apiKey}, nil
	case "2captcha":
		softID := os.Getenv("SOFT_ID")
		return &TwoCaptchaSolver{APIKey: apiKey, SoftID: softID}, nil
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
	payload := map[string]interface{}{
		"clientKey": s.APIKey,
		"EzAppID":   s.ezappID,
		"task": map[string]string{
			"type":       "ReCaptchaV2TaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
		},
	}

	return sendRequest("https://api.ez-captcha.com/createTask", payload)
}

func (s *TwoCaptchaSolver) createTask(siteKey, pageURL string) (string, error) {
	payload := map[string]interface{}{
		"clientKey": s.APIKey,
		"softId":    s.SoftID,
		"task": map[string]string{
			"type":       "ReCaptchaV2TaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
		},
	}

	return sendRequest("https://2captcha.com/in.php", payload)
}

func (s *EZCaptchaSolver) getTaskResult(taskID string) (string, error) {
	return pollForResult("https://api.ez-captcha.com/getTaskResult", s.APIKey, taskID)
}

func (s *TwoCaptchaSolver) getTaskResult(taskID string) (string, error) {
	return pollForResult("https://2captcha.com/res.php", s.APIKey, taskID)
}

func sendRequest(url string, payload interface{}) (string, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result struct {
		TaskID string `json:"taskId"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return result.TaskID, nil
}

func pollForResult(url, apiKey, taskID string) (string, error) {
	for i := 0; i < MaxRetries; i++ {
		payload := map[string]string{
			"clientKey": apiKey,
			"taskId":    taskID,
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to marshal JSON payload: %w", err)
		}

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			return "", fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}

		var result struct {
			Status   string `json:"status"`
			Solution struct {
				GRecaptchaResponse string `json:"gRecaptchaResponse"`
			} `json:"solution"`
		}
		err = json.Unmarshal(body, &result)
		if err != nil {
			return "", fmt.Errorf("failed to parse JSON response: %w", err)
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		logger.Log.Infof("Waiting for captcha solution. Attempt %d/%d", i+1, MaxRetries)
		time.Sleep(RetryInterval)
	}

	return "", errors.New("max retries reached, captcha solving timed out")
}

func ValidateCaptchaKey(apiKey, provider string) (bool, float64, error) {
	var url string
	switch provider {
	case "ezcaptcha":
		url = "https://api.ez-captcha.com/getBalance"
	case "2captcha":
		url = "https://2captcha.com/getBalance"
	default:
		return false, 0, errors.New("unsupported captcha provider")
	}

	payload := map[string]string{
		"clientKey": apiKey,
		"action":    "getBalance",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return false, 0, fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
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
