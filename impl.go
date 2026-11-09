package klient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second
const maxResponseBytes = 32 << 20

var defaultHTTPClient = &http.Client{Timeout: defaultTimeout}

var ErrAPI = errors.New("klient: api error")

type APIError struct {
	Code      int
	Msg       string
	RequestID string
	Details   json.RawMessage
}

func (e *APIError) Error() string {
	return fmt.Sprintf("klient: api error code=%d msg=%q request_id=%s", e.Code, e.Msg, e.RequestID)
}

func (e *APIError) Unwrap() error { return ErrAPI }

type envelope struct {
	Code      int             `json:"code"`
	Msg       string          `json:"msg"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
	Details   json.RawMessage `json:"details"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("klient: marshal request body: %w", err)
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.restBase()+path, rdr)
	if err != nil {
		return fmt.Errorf("klient: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = defaultHTTPClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("klient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("klient: %s %s: read response: %w", method, path, err)
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("klient: %s %s: response body exceeds %d bytes", method, path, maxResponseBytes)
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("klient: %s %s: status %d, response is not an envelope: %w",
			method, path, resp.StatusCode, err)
	}
	if env.Code != 0 {
		return &APIError{Code: env.Code, Msg: env.Msg, RequestID: env.RequestID, Details: env.Details}
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("klient: %s %s: decode data: %w", method, path, err)
		}
	}
	return nil
}

func (c *Client) restBase() string {
	return strings.TrimRight(c.BaseURL, "/")
}
