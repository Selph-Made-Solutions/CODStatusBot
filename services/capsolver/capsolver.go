package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

const capSolverAPIURL = "https://api.capsolver.com"

type CapSolverResponse struct {
	ErrorId          int32          `json:"errorId"`
	ErrorCode        string         `json:"errorCode"`
	ErrorDescription string         `json:"errorDescription"`
	TaskId           string         `json:"taskId"`
	Status           string         `json:"status"`
	Solution         map[string]any `json:"solution"`
}

func SolveReCaptcha(ctx context.Context, apiKey, websiteURL, websiteKey string) (string, error) {
	taskData := map[string]any{
		"type":       "ReCaptchaV2TaskProxyLess",
		"websiteURL": websiteURL,
		"websiteKey": websiteKey,
	}

	res, err := createCapSolverTask(ctx, apiKey, taskData)
	if err != nil {
		return "", err
	}

	solution, err := getCapSolverTaskResult(ctx, apiKey, res.TaskId)
	if err != nil {
		return "", err
	}

	gRecaptchaResponse, ok := solution["gRecaptchaResponse"].(string)
	if !ok {
		return "", errors.New("invalid gRecaptchaResponse in solution")
	}

	return gRecaptchaResponse, nil
}

func createCapSolverTask(ctx context.Context, apiKey string, taskData map[string]any) (*CapSolverResponse, error) {
	payload := map[string]any{
		"clientKey": apiKey,
		"task":      taskData,
	}

	return makeCapSolverRequest(ctx, "/createTask", payload)
}

func getCapSolverTaskResult(ctx context.Context, apiKey, taskId string) (map[string]any, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("solve timeout")
		case <-time.After(time.Second * 5):
			payload := map[string]any{
				"clientKey": apiKey,
				"taskId":    taskId,
			}

			res, err := makeCapSolverRequest(ctx, "/getTaskResult", payload)
			if err != nil {
				return nil, err
			}

			if res.Status == "ready" {
				return res.Solution, nil
			}
		}
	}
}

func makeCapSolverRequest(ctx context.Context, endpoint string, payload interface{}) (*CapSolverResponse, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", capSolverAPIURL+endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var capResponse CapSolverResponse
	err = json.Unmarshal(responseData, &capResponse)
	if err != nil {
		return nil, err
	}

	if capResponse.ErrorId != 0 {
		return nil, errors.New(capResponse.ErrorDescription)
	}

	return &capResponse, nil
}
