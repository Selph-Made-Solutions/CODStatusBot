package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/logger"
)

const (
	CapsolverCreateEndpoint   = "https://api.capsolver.com/createTask"
	CapsolverResultEndpoint   = "https://api.capsolver.com/getTaskResult"
	CapsolverFeedbackEndpoint = "https://api.capsolver.com/feedbackTask"
	EZCaptchaCreateEndpoint   = "https://api.ez-captcha.com/createTask"
	EZCaptchaResultEndpoint   = "https://api.ez-captcha.com/getTaskResult"
	TwoCaptchaCreateEndpoint  = "https://api.2captcha.com/createTask"
	TwoCaptchaResultEndpoint  = "https://api.2captcha.com/getTaskResult"
	MaxRetries                = 6
	RetryInterval             = 10 * time.Second
)

type CaptchaSolver interface {
	SolveReCaptchaV2(siteKey, pageURL string) (string, error)
}
type CapsolverSolver struct {
	APIKey string
	AppID  string
}

type EZCaptchaSolver struct {
	APIKey  string
	EzappID string
}

type TwoCaptchaSolver struct {
	APIKey string
	SoftID string
}

var capsolverTasksMutex sync.RWMutex
var capsolverTasks = make(map[string]captchaTaskInfo)

type captchaTaskInfo struct {
	token     string
	timestamp time.Time
}

func init() {
	go cleanupStaleTasks()
}

func cleanupStaleTasks() {
	for {
		time.Sleep(10 * time.Minute)

		capsolverTasksMutex.Lock()
		now := time.Now()

		staleThreshold := now.Add(-15 * time.Minute)

		var staleTasks []string
		for taskID, info := range capsolverTasks {
			if info.timestamp.Before(staleThreshold) {
				staleTasks = append(staleTasks, taskID)
			}
		}

		for _, taskID := range staleTasks {
			delete(capsolverTasks, taskID)
		}

		if len(staleTasks) > 0 {
			logger.Log.Infof("Cleaned up %d stale Capsolver tasks", len(staleTasks))
		}

		capsolverTasksMutex.Unlock()
	}
}

func StoreCapsolverTaskInfo(taskID, token string) {
	capsolverTasksMutex.Lock()
	defer capsolverTasksMutex.Unlock()
	capsolverTasks[taskID] = captchaTaskInfo{
		token:     token,
		timestamp: time.Now(),
	}
}

func ReportCapsolverTaskResult(token string, isValid bool, errorMessage string) {
	capsolverTasksMutex.RLock()
	var taskID string
	for id, info := range capsolverTasks {
		if info.token == token {
			taskID = id
			break
		}
	}
	capsolverTasksMutex.RUnlock()

	if taskID == "" {
		logger.Log.Warn("Cannot report Capsolver result: no task ID found for token")
		return
	}

	cfg := configuration.Get()

	successStatus := "failure"
	if isValid {
		successStatus = "success"
	}

	logger.Log.Infof("Reporting captcha %s to Capsolver for task %s (error: %s)",
		successStatus,
		taskID,
		errorMessage)

	reportErr := reportCapsolverTaskResult(cfg.CaptchaService.Capsolver.ClientKey, cfg.CaptchaService.Capsolver.AppID, taskID, isValid, 0, errorMessage)
	if reportErr != nil {
		logger.Log.WithError(reportErr).Warn("Failed to report Capsolver task result")
	} else {
		logger.Log.Infof("Successfully reported Capsolver task result for task %s, valid: %v", taskID, isValid)
	}

	capsolverTasksMutex.Lock()
	delete(capsolverTasks, taskID)
	capsolverTasksMutex.Unlock()
}

func reportCapsolverTaskResult(apiKey, appID, taskID string, isValid bool, errorCode int, errorMessage string) error {
	if apiKey == "" || taskID == "" {
		return errors.New("missing required parameters for feedback report")
	}

	payload := map[string]interface{}{
		"clientKey": apiKey,
		"taskId":    taskID,
		"result": map[string]interface{}{
			"invalid": !isValid,
		},
	}

	if appID != "" {
		payload["appId"] = appID
	}

	if !isValid && errorCode > 0 {
		resultMap := payload["result"].(map[string]interface{})
		resultMap["code"] = errorCode
		if errorMessage != "" {
			resultMap["message"] = errorMessage
		}
	}

	resp, err := sendRequest(CapsolverFeedbackEndpoint, payload)
	if err != nil {
		return fmt.Errorf("failed to send feedback: %w", err)
	}

	var result struct {
		ErrorId          int    `json:"errorId"`
		ErrorCode        string `json:"errorCode"`
		ErrorDescription string `json:"errorDescription"`
		Message          string `json:"message"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse feedback response: %w", err)
	}

	if result.ErrorId != 0 {
		return fmt.Errorf("feedback API error: %s - %s", result.ErrorCode, result.ErrorDescription)
	}

	logger.Log.Infof("Successfully reported Capsolver task result for task %s, valid: %v", taskID, isValid)
	return nil
}

func IsServiceEnabled(provider string) bool {
	cfg := configuration.Get()
	switch provider {
	case "capsolver":
		return cfg.CaptchaService.Capsolver.Enabled && cfg.CaptchaService.Capsolver.ClientKey != ""
	case "ezcaptcha":
		return cfg.CaptchaService.EZCaptcha.Enabled && cfg.CaptchaService.EZCaptcha.ClientKey != ""
	case "2captcha":
		return cfg.CaptchaService.TwoCaptcha.Enabled && cfg.CaptchaService.TwoCaptcha.ClientKey != ""
	default:
		return false
	}
}

func VerifyEZCaptchaConfig() bool {
	cfg := configuration.Get()
	ezConfig := cfg.CaptchaService

	if ezConfig.EZCaptcha.ClientKey == "" || ezConfig.EZCaptcha.AppID == "" {
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
	case "capsolver":
		return &CapsolverSolver{
			APIKey: apiKey,
			AppID:  cfg.CaptchaService.Capsolver.AppID,
		}, nil
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

func (s *CapsolverSolver) SolveReCaptchaV2(siteKey, pageURL string) (string, error) {
	taskID, err := s.createTask(siteKey, pageURL)
	if err != nil {
		return "", fmt.Errorf("failed to create capsolver task: %w", err)
	}

	response, err := s.getTaskResult(taskID)
	if err != nil {
		if !strings.Contains(err.Error(), "failed to send request") &&
			!strings.Contains(err.Error(), "failed to read response") {
			logger.Log.Infof("Reporting Capsolver task failure for task %s", taskID)
			reportErr := reportCapsolverTaskResult(s.APIKey, s.AppID, taskID, false, 1001, err.Error())
			if reportErr != nil {
				logger.Log.WithError(reportErr).Warn("Failed to report Capsolver task failure")
			}
		}
		return "", err
	}

	StoreCapsolverTaskInfo(taskID, response)

	return response, nil
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

func (s *CapsolverSolver) createTask(siteKey, pageURL string) (string, error) {

	if s.AppID == "" {
		return "", fmt.Errorf("AppID is not configured")
	}

	payload := map[string]interface{}{
		"clientKey": s.APIKey,
		"appId":     s.AppID,
		"task": map[string]interface{}{
			"type":        "ReCaptchaV2EnterpriseTaskProxyless",
			"websiteURL":  pageURL,
			"websiteKey":  siteKey,
			"isInvisible": false,
		},
	}

	resp, err := sendRequest(CapsolverCreateEndpoint, payload)
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
		return "", fmt.Errorf("failed to parse capsolver response: %w", err)
	}

	if result.ErrorId != 0 {
		return "", fmt.Errorf("capsolver API error: %s - %s", result.ErrorCode, result.ErrorDescription)
	}

	return result.TaskId, nil
}

func (s *EZCaptchaSolver) createTask(siteKey, pageURL string) (string, error) {

	if s.EzappID == "" {
		return "", fmt.Errorf("EzappID is not configured")
	}

	payload := map[string]interface{}{
		"clientKey": s.APIKey,
		"appId":     s.EzappID,
		"task": map[string]interface{}{
			"type":        "ReCaptchaV2EnterpriseTaskProxyless",
			"websiteURL":  pageURL,
			"websiteKey":  siteKey,
			"isInvisible": false,
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

	if s.SoftID == "" {
		return "", fmt.Errorf("SoftID is not configured")
	}

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

func (s *CapsolverSolver) getTaskResult(taskID string) (string, error) {
	cfg := configuration.Get()
	MaxRetries := cfg.CaptchaService.Capsolver.MaxRetries
	RetryInterval := cfg.CaptchaService.Capsolver.RetryInterval

	for i := 0; i < MaxRetries; i++ {
		payload := map[string]interface{}{
			"clientKey": s.APIKey,
			"taskId":    taskID,
		}

		resp, err := sendRequest(CapsolverResultEndpoint, payload)
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
			return "", fmt.Errorf("failed to parse capsolver response: %w", err)
		}

		if result.ErrorId != 0 {
			if strings.Contains(result.ErrorDescription, "insufficient balance") {
				return "", fmt.Errorf("insufficient balance")
			}
			if i == MaxRetries-1 {
				return "", fmt.Errorf("capsolver API error after %d retries: %s - %s", MaxRetries, result.ErrorCode, result.ErrorDescription)
			}
			time.Sleep(RetryInterval)
			continue
		}

		if result.Status == "ready" {
			if len(result.Solution.GRecaptchaResponse) < 50 {
				return "", fmt.Errorf("invalid captcha response received from capsolver")
			}
			return result.Solution.GRecaptchaResponse, nil
		}

		time.Sleep(RetryInterval)
	}

	return "", fmt.Errorf("max retries reached (%d) waiting for capsolver result", MaxRetries)

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
			if strings.Contains(result.ErrorDescription, "insufficient balance") {
				return "", fmt.Errorf("insufficient balance")
			}
			if i == MaxRetries-1 {
				return "", fmt.Errorf("API error after %d retries: %s - %s", MaxRetries, result.ErrorCode, result.ErrorDescription)
			}
			time.Sleep(RetryInterval)
			continue
		}

		if result.Status == "ready" {
			if len(result.Solution.GRecaptchaResponse) < 50 {
				return "", fmt.Errorf("invalid captcha response received")
			}
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
	case "capsolver":
		return cfg.CaptchaService.Capsolver.BalanceMin
	case "ezcaptcha":
		return cfg.CaptchaService.EZCaptcha.BalanceMin
	case "2captcha":
		return cfg.CaptchaService.TwoCaptcha.BalanceMin
	default:
		return 0
	}
}

func ValidateCaptchaKey(apiKey, provider string) (bool, float64, error) {
	if apiKey == "" {
		return false, 0, fmt.Errorf("empty API key provided")
	}

	switch provider {
	case "capsolver":
		return validateCapsolverKey(apiKey)
	case "ezcaptcha":
		return validateEZCaptchaKey(apiKey)
	case "2captcha":
		return validate2CaptchaKey(apiKey)
	default:
		return false, 0, errors.New("unsupported captcha provider")
	}
}

func validateCapsolverKey(apiKey string) (bool, float64, error) {
	url := "https://api.capsolver.com/getBalance"
	payload := map[string]string{
		"clientKey": apiKey,
	}

	resp, err := sendRequest(url, payload)
	if err != nil {
		if strings.Contains(err.Error(), "invalid token format") {
			return false, 0, errors.New("invalid capsolver API key format")
		}
		if strings.Contains(err.Error(), "invalid key") {
			return false, 0, errors.New("invalid capsolver API key")
		}
		return false, 0, fmt.Errorf("capsolver balance check failed: %w", err)
	}

	var result struct {
		ErrorId          int     `json:"errorId"`
		ErrorCode        string  `json:"errorCode"`
		ErrorDescription string  `json:"errorDescription"`
		Balance          float64 `json:"balance"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return false, 0, fmt.Errorf("failed to parse capsolver response: %w", err)
	}

	if result.ErrorId != 0 {
		if strings.Contains(result.ErrorDescription, "Invalid token format") {
			return false, 0, errors.New("invalid capsolver API key format")
		}
		if strings.Contains(result.ErrorDescription, "Invalid key") {
			return false, 0, errors.New("invalid capsolver API key")
		}
		return false, 0, fmt.Errorf("capsolver API error: %s - %s", result.ErrorCode, result.ErrorDescription)
	}

	return true, result.Balance, nil
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
