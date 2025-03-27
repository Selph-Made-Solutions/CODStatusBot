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

	defaultClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	longTimeoutClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	clientsInitialized = true
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
