package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	locoFallback = "auto"
	locoFilter   = "filter"
)

func isValidDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func locoRequest(apiKey, URL string, queryParams url.Values) (resp *http.Response, err error) {
	reqURL, err := url.Parse(URL)
	if err != nil {
		return nil, err
	}
	reqURL.RawQuery = queryParams.Encode()

	client := http.DefaultClient
	client.Timeout = 30 * time.Second
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	slog.Info(reqURL.String())
	if err != nil {
		return
	}
	req.Header.Add(authHeader, fmt.Sprintf("Loco %s", apiKey))
	resp, err = client.Do(req)
	if err != nil {
		slog.Error("error fetching", slog.Any("err", err))
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("status not OK: is %d", resp.StatusCode)
	}
	return
}

func locoWrite(apiKey, URL, method string, body []byte) (resp *http.Response, err error) {
	reqURL, err := url.Parse(URL)
	if err != nil {
		return
	}

	client := http.DefaultClient
	client.Timeout = 30 * time.Second
	req, err := http.NewRequest(method, reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Add(authHeader, fmt.Sprintf("Loco %s", apiKey))

	resp, err = client.Do(req)
	if err != nil {
		slog.Error("error performing", slog.Any("err", err))
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("status not OK: is %d", resp.StatusCode)
	}
	return
}
