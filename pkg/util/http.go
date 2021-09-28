package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

type HttpClient struct {
	client *http.Client
}

type ReqOpts struct {
	Method      string
	Url         string
	JsonContent interface{}
}

// Could make client options tune-able.
func NewHttpClient() *HttpClient {
	return &HttpClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout: 10 * time.Second,
				}).Dial,
				IdleConnTimeout: 30 * time.Second,
			},
		},
	}
}

func (h *HttpClient) Send(reqOpts ReqOpts) ([]byte, error) {
	if reqOpts.Method == "" || reqOpts.Url == "" {
		return nil, fmt.Errorf("invalid request options: %+v", reqOpts)
	}

	var reader io.Reader
	if reqOpts.JsonContent != nil {
		payload, err := json.Marshal(reqOpts.JsonContent)
		if err != nil {
			return nil, err
		}

		reader = bytes.NewBuffer(payload)
	} else {
		reader = nil
	}

	req, err := http.NewRequest(reqOpts.Method, reqOpts.Url, reader)
	if err != nil {
		return nil, err
	}

	if reqOpts.JsonContent != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("received HTTP status %d with body: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func NewHttpServer(mux *http.ServeMux) *http.Server {
	return &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		Addr:         fmt.Sprintf(":%s", LoadEnvString("PORT", "8080")),
		Handler:      http.TimeoutHandler(mux, 30*time.Second, "Timeout"),
	}
}
