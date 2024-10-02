package nextcaptcha

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

const (
	HOST                      = "https://api.nextcaptcha.com"
	TIMEOUT                   = 45 * time.Second
	PendingStatus             = "pending"
	ProcessingStatus          = "processing"
	ReadyStatus               = "ready"
	FailedStatus              = "failed"
	Recaptchav2Type           = "RecaptchaV2TaskProxyless"
	Recaptchav2EnterpriseType = "RecaptchaV2EnterpriseTaskProxyless"
	Recaptchav3ProxylessType  = "RecaptchaV3TaskProxyless"
	Recaptchav3Type           = "RecaptchaV3Task"
	RecaptchaMobileType       = "RecaptchaMobileProxyless"
	HcaptchaType              = "HCaptchaTask"
	HcaptchaProxylessType     = "HCaptchaTaskProxyless"
	HcaptchaEnterpriseType    = "HCaptchaEnterpriseTask"
	FuncaptchaType            = "FunCaptchaTask"
	FuncaptchaProxylessType   = "FunCaptchaTaskProxyless"
)

type TaskBadParametersError struct {
	s string
}

func (e *TaskBadParametersError) Error() string {
	return e.s
}

type ApiClient struct {
	clientKey   string
	solftID     string
	callbackURL string
	openLog     bool
	httpClient  *http.Client
}

func NewApiClient(clientKey, solftId, callbackUrl string, openLog bool) *ApiClient {
	return &ApiClient{
		clientKey:   clientKey,
		solftID:     solftId,
		callbackURL: callbackUrl,
		openLog:     openLog,
		httpClient:  &http.Client{Timeout: TIMEOUT},
	}
}

func (c *ApiClient) getBalance() (string, error) {
	data := map[string]string{"clientKey": c.clientKey}
	resp, err := c.postJSON("/getBalance", data)
	if err != nil {
		if c.openLog {
			log.Printf("Error: %v", err)
		}
		return "", err
	}

	balance := resp["balance"].(string)
	if c.openLog {
		log.Printf("Balance: %s", balance)
	}

	return balance, nil
}

func (c *ApiClient) send(task map[string]interface{}) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"clientKey":   c.clientKey,
		"softId":     c.solftID,
		"callbackUrl": c.callbackURL,
		"task":        task,
	}
	resp, err := c.postJSON("/createTask", data)
	if err != nil {
		if c.openLog {
			log.Printf("Error: %v", err)
			log.Printf("Data: %v", data)
		}
		return nil, err
	}

	taskID := resp["taskId"].(float64)
	if c.openLog {
		log.Printf("Task %f created %v", taskID, resp)
	}

	startTime := time.Now()
	for {
		if time.Since(startTime) > TIMEOUT {
			return map[string]interface{}{
				"errorId":          12,
				"errorDescription": "Timeout",
				"status":           "failed",
			}, nil
		}

		data := map[string]any{
			"clientKey": c.clientKey,
			"taskId":    taskID,
		}
		resp, err := c.postJSON("/getTaskResult", data)
		if err != nil {
			if c.openLog {
				log.Printf("Error: %v", err)
			}
			return nil, err
		}

		status := resp["status"].(string)
		if c.openLog {
			log.Printf("Task status: %s", status)
		}

		if status == ReadyStatus {
			log.Printf("Task %f ready %v", taskID, resp)
			return resp, nil
		}
		if status == FailedStatus {
			log.Printf("Task %f failed %v", taskID, resp)
			return resp, nil
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *ApiClient) postJSON(path string, data interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, HOST+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		if c.openLog {
			log.Printf("Error: %d %s", resp.StatusCode, string(body))
		}
		return nil, errors.New(string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

type NextCaptchaAPI struct {
	api *ApiClient
}

func NewNextCaptchaAPI(clientKey, solftId, callbackUrl string, openLog bool) *NextCaptchaAPI {
	log.Printf("NextCaptchaAPI created with clientKey=%s solftId=%s callbackUrl=%s", clientKey, solftId, callbackUrl)
	api := NewApiClient(clientKey, solftId, callbackUrl, openLog)
	return &NextCaptchaAPI{api}
}

type RecaptchaV2Options struct {
	RecaptchaDataSValue string
	IsInvisible         bool
	ApiDomain           string
	PageAction          string
}

func (api *NextCaptchaAPI) RecaptchaV2(websiteURL, websiteKey string, options RecaptchaV2Options) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":       Recaptchav2Type,
		"websiteURL": websiteURL,
		"websiteKey": websiteKey,
	}
	if options.RecaptchaDataSValue != "" {
		task["recaptchaDataSValue"] = options.RecaptchaDataSValue
	}
	if options.IsInvisible {
		task["isInvisible"] = options.IsInvisible
	}
	if options.ApiDomain != "" {
		task["apiDomain"] = options.ApiDomain
	}
	if options.PageAction != "" {
		task["pageAction"] = options.PageAction
	}
	return api.api.send(task)
}

type RecaptchaV2EnterpriseOptions struct {
	EnterprisePayload map[string]interface{}
	IsInvisible       bool
	ApiDomain         string
	PageAction        string
}

func (api *NextCaptchaAPI) RecaptchaV2Enterprise(websiteURL, websiteKey string, options RecaptchaV2EnterpriseOptions) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":       Recaptchav2EnterpriseType,
		"websiteURL": websiteURL,
		"websiteKey": websiteKey,
	}
	if options.EnterprisePayload != nil {
		task["enterprisePayload"] = options.EnterprisePayload
	}
	if options.IsInvisible {
		task["isInvisible"] = options.IsInvisible
	}
	if options.ApiDomain != "" {
		task["apiDomain"] = options.ApiDomain
	}
	if options.PageAction != "" {
		task["pageAction"] = options.PageAction
	}
	return api.api.send(task)
}

type RecaptchaV3Options struct {
	PageAction string
	APIDomain  string
	ProxyType  string
	ProxyAddress  string
	ProxyPort     int
	ProxyLogin    string
	ProxyPassword string
}

func (api *NextCaptchaAPI) RecaptchaV3(websiteURL, websiteKey string, options RecaptchaV3Options) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":       Recaptchav3ProxylessType,
		"websiteURL": websiteURL,
		"websiteKey": websiteKey,
	}
	if options.PageAction != "" {
		task["pageAction"] = options.PageAction
	}
	if options.APIDomain != "" {
		task["apiDomain"] = options.APIDomain
	}
	if options.ProxyAddress != "" {
		task["type"] = Recaptchav3Type
		task["proxyType"] = options.ProxyType
		task["proxyAddress"] = options.ProxyAddress
		task["proxyPort"] = options.ProxyPort
		task["proxyLogin"] = options.ProxyLogin
		task["proxyPassword"] = options.ProxyPassword
	}
	return api.api.send(task)
}

type RecaptchaMobileOptions struct {
	AppPackageName string
	AppAction      string
}

func (api *NextCaptchaAPI) RecaptchaMobile(appKey string, options RecaptchaMobileOptions) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":   RecaptchaMobileType,
		"appKey": appKey,
	}
	if options.AppPackageName != "" {
		task["appPackageName"] = options.AppPackageName
	}
	if options.AppAction != "" {
		task["appAction"] = options.AppAction
	}
	return api.api.send(task)
}

type HCaptchaOptions struct {
	IsInvisible       bool
	EnterprisePayload map[string]interface{}
	ProxyType         string
	ProxyAddress      string
	ProxyPort         int
	ProxyLogin        string
	ProxyPassword     string
}

func (api *NextCaptchaAPI) HCaptcha(websiteURL, websiteKey string, options HCaptchaOptions) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":       HcaptchaProxylessType,
		"websiteURL": websiteURL,
		"websiteKey": websiteKey,
	}
	if options.IsInvisible {
		task["isInvisible"] = options.IsInvisible
	}
	if options.EnterprisePayload != nil {
		task["enterprisePayload"] = options.EnterprisePayload
	}
	if options.ProxyAddress != "" {
		task["type"] = HcaptchaType
		task["proxyType"] = options.ProxyType
		task["proxyAddress"] = options.ProxyAddress
		task["proxyPort"] = options.ProxyPort
		task["proxyLogin"] = options.ProxyLogin
		task["proxyPassword"] = options.ProxyPassword
	}
	return api.api.send(task)
}

type HCaptchaEnterpriseOptions struct {
	EnterprisePayload map[string]interface{}
	IsInvisible       bool
	ProxyType         string
	ProxyAddress      string
	ProxyPort         int
	ProxyLogin        string
	ProxyPassword     string
}

func (api *NextCaptchaAPI) HCaptchaEnterprise(websiteURL, websiteKey string, options HCaptchaEnterpriseOptions) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":       HcaptchaEnterpriseType,
		"websiteURL": websiteURL,
		"websiteKey": websiteKey,
	}
	if options.EnterprisePayload != nil {
		task["enterprisePayload"] = options.EnterprisePayload
	}
	if options.IsInvisible {
		task["isInvisible"] = options.IsInvisible
	}
	task["proxyType"] = options.ProxyType
	task["proxyAddress"] = options.ProxyAddress
	task["proxyPort"] = options.ProxyPort
	task["proxyLogin"] = options.ProxyLogin
	task["proxyPassword"] = options.ProxyPassword
	return api.api.send(task)
}

type FunCaptchaOptions struct {
	WebsiteURL    string
	Data          string
	ProxyType     string
	ProxyAddress  string
	ProxyPort     int
	ProxyLogin    string
	ProxyPassword string
}

func (api *NextCaptchaAPI) FunCaptcha(websitePublicKey string, options FunCaptchaOptions) (map[string]interface{}, error) {
	task := map[string]interface{}{
		"type":             FuncaptchaProxylessType,
		"websitePublicKey": websitePublicKey,
	}
	if options.WebsiteURL != "" {
		task["websiteURL"] = options.WebsiteURL
	}
	if options.Data != "" {
		task["data"] = options.Data
	}
	if options.ProxyAddress != "" {
		task["type"] = FuncaptchaType
		task["proxyType"] = options.ProxyType
		task["proxyAddress"] = options.ProxyAddress
		task["proxyPort"] = options.ProxyPort
		task["proxyLogin"] = options.ProxyLogin
		task["proxyPassword"] = options.ProxyPassword
	}
	return api.api.send(task)
}
