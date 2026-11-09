package klient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	BaseURL       string
	Token         string
	HTTPClient    *http.Client
	DialTimeout   time.Duration
	MaxFrameBytes int64
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: baseURL, Token: token, DialTimeout: 10 * time.Second}
}

type Session struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Title       string `json:"title,omitempty"`
}

type Snapshot struct {
	AsOfSeq int64  `json:"as_of_seq"`
	Epoch   string `json:"epoch"`
	Session struct {
		Busy           bool `json:"busy"`
		MainTurnActive bool `json:"main_turn_active"`
	} `json:"session"`
	InFlightTurn *InFlightTurn `json:"in_flight_turn"`
	Messages     struct {
		Items []struct {
			Metadata struct {
				Origin struct {
					Kind   string `json:"kind"`
					TaskID string `json:"taskId"`
				} `json:"origin"`
			} `json:"metadata"`
		} `json:"items"`
	} `json:"messages"`
}

type InFlightTurn struct {
	TurnID          int64  `json:"turn_id"`
	CurrentPromptID string `json:"current_prompt_id"`
}

type Task struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	RunInBackground bool   `json:"run_in_background"`
	CompletedAt     string `json:"completed_at"`
}

func (c *Client) ListTasks(ctx context.Context, sessionID string) ([]Task, error) {
	var data struct {
		Items []Task `json:"items"`
	}
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/tasks"
	if err := c.do(ctx, http.MethodGet, path, nil, &data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func (s *Snapshot) Cursor() Cursor {
	return Cursor{Seq: s.AsOfSeq, Epoch: s.Epoch}
}

type PromptReceipt struct {
	PromptID string `json:"prompt_id"`
}

type MCPServer struct {
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"`
	Status    string `json:"status"`
	ToolCount int    `json:"tool_count,omitempty"`
}

func (c *Client) ListMCPServers(ctx context.Context, sessionID string) ([]MCPServer, error) {
	var data struct {
		Servers []MCPServer `json:"servers"`
	}
	path := "/api/v1/mcp/servers?session_id=" + url.QueryEscape(sessionID)
	if err := c.do(ctx, http.MethodGet, path, nil, &data); err != nil {
		return nil, err
	}
	return data.Servers, nil
}

func (c *Client) Healthz(ctx context.Context) error {
	var data struct {
		OK bool `json:"ok"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/healthz", nil, &data); err != nil {
		return err
	}
	if !data.OK {
		return errors.New("klient: healthz reports not ok")
	}
	return nil
}

func (c *Client) CreateSession(ctx context.Context, cwd string) (*Session, error) {
	body := map[string]any{"metadata": map[string]string{"cwd": cwd}}
	var session Session
	if err := c.do(ctx, http.MethodPost, "/api/v1/sessions", body, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) GetSnapshot(ctx context.Context, sessionID string) (*Snapshot, error) {
	var snapshot Snapshot
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/snapshot"
	if err := c.do(ctx, http.MethodGet, path, nil, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

type PromptRequest struct {
	Text           string
	Model          string
	PermissionMode string
}

func (c *Client) PostPrompt(ctx context.Context, sessionID string, preq PromptRequest) (*PromptReceipt, error) {
	body := map[string]any{
		"content": []map[string]string{{"type": "text", "text": preq.Text}},
	}
	if preq.Model != "" {
		body["model"] = preq.Model
	}
	if preq.PermissionMode != "" {
		body["permission_mode"] = preq.PermissionMode
	}
	var receipt PromptReceipt
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/prompts"
	if err := c.do(ctx, http.MethodPost, path, body, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (c *Client) Abort(ctx context.Context, sessionID string) error {
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + ":abort"
	return c.do(ctx, http.MethodPost, path, struct{}{}, nil)
}

func (c *Client) SteerPrompts(ctx context.Context, sessionID string, promptIDs []string) error {
	for _, id := range promptIDs {
		path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/prompts/" + url.PathEscape(id) + ":steer"
		if err := c.do(ctx, http.MethodPost, path, struct{}{}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Subscribe(ctx context.Context, sessionID string, cursor Cursor) (*Subscription, error) {
	return c.subscribe(ctx, sessionID, cursor)
}
