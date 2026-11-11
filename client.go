package klient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Client struct {
	BaseURL       string
	Token         string
	HTTPClient    *http.Client
	DialTimeout   time.Duration
	MaxFrameBytes int64

	streamMu sync.Mutex
	streams  map[string]*transcriptEventMapper
	active   map[string]context.CancelFunc
	released map[string]bool
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: baseURL, Token: token, DialTimeout: 10 * time.Second}
}

type Session struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspace_id,omitempty"`
	Title              string         `json:"title,omitempty"`
	CreatedAt          string         `json:"created_at,omitempty"`
	UpdatedAt          string         `json:"updated_at,omitempty"`
	ArchivedAt         string         `json:"archived_at,omitempty"`
	Busy               bool           `json:"busy"`
	MainTurnActive     bool           `json:"main_turn_active,omitempty"`
	PendingInteraction string         `json:"pending_interaction,omitempty"`
	LastTurnReason     string         `json:"last_turn_reason,omitempty"`
	Archived           bool           `json:"archived,omitempty"`
	CurrentPromptID    string         `json:"current_prompt_id,omitempty"`
	LastPrompt         string         `json:"last_prompt,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	AgentConfig        struct {
		Model string `json:"model,omitempty"`
	} `json:"agent_config"`
	Usage        SessionUsage `json:"usage"`
	MessageCount int64        `json:"message_count,omitempty"`
}

type SessionUsage struct {
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	TotalCostUSD        float64 `json:"total_cost_usd,omitempty"`
	ContextTokens       int64   `json:"context_tokens"`
	ContextLimit        int64   `json:"context_limit,omitempty"`
	TurnCount           int64   `json:"turn_count,omitempty"`
}

type Snapshot struct {
	AgentID             string             `json:"agent_id"`
	Items               []TranscriptItem   `json:"items"`
	HasMore             bool               `json:"has_more"`
	Interactions        []json.RawMessage  `json:"interactions"`
	Attachments         []json.RawMessage  `json:"attachments"`
	Prompts             []TranscriptPrompt `json:"prompts"`
	Agents              []AgentDescriptor  `json:"agents"`
	PendingInteractions []string           `json:"pending_interactions"`
	Seq                 *int64             `json:"seq"`
	Session             Session            `json:"-"`
	InFlightTurn        *InFlightTurn      `json:"-"`
}

type InFlightTurn struct {
	TurnID          string `json:"turn_id"`
	CurrentPromptID string `json:"current_prompt_id"`
}

func (s *Snapshot) Cursor() Cursor {
	cursor := Cursor{TranscriptSince: make(map[string]int64)}
	if s.Seq != nil {
		cursor.TranscriptSince[s.AgentID] = *s.Seq
		cursor.baseline = map[string]transcriptSnapshot{
			s.AgentID: {
				Items:        s.Items,
				Interactions: s.Interactions,
				Attachments:  s.Attachments,
				Prompts:      s.Prompts,
				HasMoreOlder: s.HasMore,
			},
		}
	}
	return cursor
}

type PromptReceipt struct {
	PromptID string `json:"prompt_id"`
}

type MCPServer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Transport   string `json:"transport,omitempty"`
	Status      string `json:"status"`
	LastError   string `json:"last_error,omitempty"`
	ToolCount   int    `json:"tool_count,omitempty"`
	NextRetryAt int64  `json:"next_retry_at,omitempty"`
}

func (c *Client) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	var data struct {
		Servers []MCPServer `json:"servers"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/mcp/servers", nil, &data); err != nil {
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

func (c *Client) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var session Session
	path := "/api/v1/sessions/" + url.PathEscape(sessionID)
	if err := c.do(ctx, http.MethodGet, path, nil, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) GetSnapshot(ctx context.Context, sessionID string) (*Snapshot, error) {
	session, err := c.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := c.GetAgentSnapshot(ctx, sessionID, "main")
	if err != nil {
		return nil, err
	}
	snapshot.Session = *session
	snapshot.setInFlightTurn()
	return snapshot, nil
}

func (c *Client) GetAgentSnapshot(ctx context.Context, sessionID, agentID string) (*Snapshot, error) {
	var snapshot Snapshot
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/transcript?agent_id=" + url.QueryEscape(agentID) + "&page_size=100"
	if err := c.do(ctx, http.MethodGet, path, nil, &snapshot); err != nil {
		return nil, err
	}
	snapshot.setInFlightTurn()
	return &snapshot, nil
}

func (s *Snapshot) setInFlightTurn() {
	s.InFlightTurn = nil
	for i := len(s.Items) - 1; i >= 0; i-- {
		item := s.Items[i]
		if item.Kind != "turn" || item.State != "running" {
			continue
		}
		promptID := item.TriggerPromptID
		if promptID == "" {
			promptID = s.Session.CurrentPromptID
		}
		s.InFlightTurn = &InFlightTurn{TurnID: item.TurnID, CurrentPromptID: promptID}
		return
	}
}

func (c *Client) RestoreSession(ctx context.Context, sessionID string) (*Session, error) {
	return c.sessionAction(ctx, sessionID, "restore")
}

func (c *Client) ReloadSession(ctx context.Context, sessionID string) (*Session, error) {
	return c.sessionAction(ctx, sessionID, "reload")
}

func (c *Client) sessionAction(ctx context.Context, sessionID, action string) (*Session, error) {
	var session Session
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + ":" + action
	if err := c.do(ctx, http.MethodPost, path, struct{}{}, &session); err != nil {
		return nil, err
	}
	return &session, nil
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
	if len(promptIDs) == 0 {
		return nil
	}
	body := map[string]any{"prompt_ids": promptIDs}
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/prompts::steer"
	return c.do(ctx, http.MethodPost, path, body, nil)
}

func (c *Client) Subscribe(ctx context.Context, sessionID string, cursor Cursor) (*Subscription, error) {
	return c.subscribe(ctx, sessionID, cursor)
}

func (c *Client) takeTranscriptMapper(sessionID string, cursor Cursor) (*transcriptEventMapper, error) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if _, exists := c.active[sessionID]; exists {
		return nil, ErrSessionSubscribed
	}
	if c.active == nil {
		c.active = make(map[string]context.CancelFunc)
	}
	c.active[sessionID] = nil
	delete(c.released, sessionID)
	mapper := c.streams[sessionID]
	delete(c.streams, sessionID)
	if mapper == nil || !mapper.matches(cursor) {
		return newTranscriptEventMapper(sessionID, cursor), nil
	}
	return mapper, nil
}

func (c *Client) activateTranscriptMapper(sessionID string, cancel context.CancelFunc) bool {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.released[sessionID] {
		return false
	}
	if _, exists := c.active[sessionID]; !exists {
		return false
	}
	c.active[sessionID] = cancel
	return true
}

func (c *Client) putTranscriptMapper(sessionID string, mapper *transcriptEventMapper) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	delete(c.active, sessionID)
	if c.released[sessionID] {
		delete(c.released, sessionID)
		delete(c.streams, sessionID)
		return
	}
	if c.streams == nil {
		c.streams = make(map[string]*transcriptEventMapper)
	}
	c.streams[sessionID] = mapper
}

func (c *Client) CloseSession(sessionID string) {
	c.streamMu.Lock()
	cancel := c.active[sessionID]
	if c.released == nil {
		c.released = make(map[string]bool)
	}
	c.released[sessionID] = true
	delete(c.streams, sessionID)
	c.streamMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
