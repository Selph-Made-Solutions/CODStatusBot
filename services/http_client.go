package services

import (
	"net/http"
	"sync"
	"time"
)

var (
	defaultClient      *http.Client
	longTimeoutClient  *http.Client
	clientMutex        sync.RWMutex
	clientsInitialized bool
)

func InitHTTPClients() {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	if clientsInitialized {
		return
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
		MaxConnsPerHost:     0,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	defaultRoundTripper := &defaultHeaderTransport{
		base: transport,
	}

	defaultClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: defaultRoundTripper,
	}

	longTimeoutClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: defaultRoundTripper,
	}

	clientsInitialized = true
}

type defaultHeaderTransport struct {
	base http.RoundTripper
}

func (t *defaultHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
	}

	return t.base.RoundTrip(req)
}

func GetDefaultHTTPClient() *http.Client {
	clientMutex.RLock()
	defer clientMutex.RUnlock()

	if !clientsInitialized {
		clientMutex.RUnlock()
		InitHTTPClients()
		clientMutex.RLock()
	}

	return defaultClient
}

func GetLongTimeoutHTTPClient() *http.Client {
	clientMutex.RLock()
	defer clientMutex.RUnlock()

	if !clientsInitialized {
		clientMutex.RUnlock()
		InitHTTPClients()
		clientMutex.RLock()
	}

	return longTimeoutClient
}
