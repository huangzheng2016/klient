package klient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type recordedRequest struct {
	Method        string
	Path          string
	RawQuery      string
	Authorization string
	ContentType   string
	Body          string
}

func restServer(t *testing.T, responses map[string]string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		rec.Method = r.Method
		rec.Path = r.URL.Path
		rec.RawQuery = r.URL.RawQuery
		rec.Authorization = r.Header.Get("Authorization")
		rec.ContentType = r.Header.Get("Content-Type")
		rec.Body = string(body)
		resp, ok := responses[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestHealthz(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"GET /api/v1/healthz": `{"code":0,"msg":"success","data":{"ok":true},"request_id":"r0"}`,
	})
	c := New(srv.URL, "tok")
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz: %v", err)
	}
	if rec.Method != http.MethodGet || rec.Path != "/api/v1/healthz" {
		t.Fatalf("request = %s %s", rec.Method, rec.Path)
	}
	if rec.Authorization != "Bearer tok" {
		t.Fatalf("Authorization = %q", rec.Authorization)
	}
}

func TestHealthzNotOK(t *testing.T) {
	srv, _ := restServer(t, map[string]string{
		"GET /api/v1/healthz": `{"code":0,"msg":"success","data":{"ok":false},"request_id":"r0"}`,
	})
	if err := New(srv.URL, "").Healthz(context.Background()); err == nil {
		t.Fatal("expected error for ok=false")
	}
}

func TestCreateSession(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"POST /api/v1/sessions": `{"code":0,"msg":"success","data":{"id":"s1","workspace_id":"w1","title":"t1","busy":false},"request_id":"r1"}`,
	})
	c := New(srv.URL, "tok")
	session, err := c.CreateSession(context.Background(), "/work/dir")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID != "s1" || session.WorkspaceID != "w1" || session.Title != "t1" {
		t.Fatalf("session = %+v", session)
	}
	if rec.Method != http.MethodPost || rec.Path != "/api/v1/sessions" {
		t.Fatalf("request = %s %s", rec.Method, rec.Path)
	}
	if rec.Authorization != "Bearer tok" {
		t.Fatalf("Authorization = %q", rec.Authorization)
	}
	if rec.ContentType != "application/json" {
		t.Fatalf("Content-Type = %q", rec.ContentType)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal([]byte(rec.Body), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["metadata"]["cwd"] != "/work/dir" {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestGetSnapshot(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"GET /api/v1/sessions/s1":            `{"code":0,"msg":"success","data":{"id":"s1","workspace_id":"w1","title":"t1","busy":true,"main_turn_active":true,"current_prompt_id":"p1","metadata":{"cwd":"/work"},"agent_config":{"model":"k2"},"usage":{"input_tokens":0,"output_tokens":0,"cache_read_tokens":0,"cache_creation_tokens":0,"context_tokens":0}},"request_id":"r2"}`,
		"GET /api/v1/sessions/s1/transcript": `{"code":0,"msg":"success","data":{"agent_id":"main","items":[{"kind":"turn","turnId":"t4","triggerPromptId":"p1","ordinal":4,"state":"running","origin":{"kind":"user"},"steps":[]}],"has_more":false,"interactions":[],"attachments":[],"prompts":[],"agents":[{"agentId":"main","type":"main"}],"pending_interactions":[],"seq":42},"request_id":"r3"}`,
	})
	c := New(srv.URL, "")
	snap, err := c.GetSnapshot(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.Seq == nil || *snap.Seq != 42 || !snap.Session.Busy || !snap.Session.MainTurnActive {
		t.Fatalf("snapshot = %+v", snap)
	}
	if snap.InFlightTurn == nil || snap.InFlightTurn.TurnID != "t4" || snap.InFlightTurn.CurrentPromptID != "p1" {
		t.Fatalf("in-flight turn = %+v", snap.InFlightTurn)
	}
	if cur := snap.Cursor(); cur.TranscriptSince["main"] != 42 {
		t.Fatalf("cursor = %+v", cur)
	}
	if rec.Path != "/api/v1/sessions/s1/transcript" || rec.RawQuery != "agent_id=main&page_size=100" {
		t.Fatalf("transcript request = %s?%s", rec.Path, rec.RawQuery)
	}
	if rec.Authorization != "" {
		t.Fatalf("Authorization = %q, want empty", rec.Authorization)
	}
}

func TestGetAgentSnapshot(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"GET /api/v1/sessions/s1/transcript": `{"code":0,"data":{"agent_id":"agent-0","items":[],"has_more":false,"interactions":[],"attachments":[],"prompts":[],"agents":[{"agentId":"main","type":"main"},{"agentId":"agent-0","type":"sub"}],"pending_interactions":[],"seq":7}}`,
	})
	snap, err := New(srv.URL, "").GetAgentSnapshot(context.Background(), "s1", "agent-0")
	if err != nil {
		t.Fatalf("GetAgentSnapshot: %v", err)
	}
	if snap.AgentID != "agent-0" || snap.Cursor().TranscriptSince["agent-0"] != 7 || len(snap.Agents) != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if rec.RawQuery != "agent_id=agent-0&page_size=100" {
		t.Fatalf("query = %q", rec.RawQuery)
	}
}

func TestPostPrompt(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"POST /api/v1/sessions/s1/prompts": `{"code":0,"msg":"success","data":{"prompt_id":"p9","user_message_id":"m9","status":"running","content":[{"type":"text","text":"hi"}],"created_at":"2026-01-01T00:00:00Z"},"request_id":"r3"}`,
	})
	c := New(srv.URL, "tok")
	receipt, err := c.PostPrompt(context.Background(), "s1", PromptRequest{Text: "hi", Model: "kimi-k3", PermissionMode: "yolo"})
	if err != nil {
		t.Fatalf("PostPrompt: %v", err)
	}
	if receipt.PromptID != "p9" {
		t.Fatalf("receipt = %+v", receipt)
	}
	var body struct {
		Model          string `json:"model"`
		PermissionMode string `json:"permission_mode"`
		Content        []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(rec.Body), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body.Model != "kimi-k3" || body.PermissionMode != "yolo" {
		t.Fatalf("model/permission_mode fields = %s", rec.Body)
	}
	if len(body.Content) != 1 || body.Content[0].Type != "text" || body.Content[0].Text != "hi" {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestAbort(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"POST /api/v1/sessions/s1:abort": `{"code":0,"msg":"success","data":{"aborted":true},"request_id":"r4"}`,
	})
	c := New(srv.URL, "tok")
	if err := c.Abort(context.Background(), "s1"); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if rec.Method != http.MethodPost || rec.Path != "/api/v1/sessions/s1:abort" {
		t.Fatalf("request = %s %s", rec.Method, rec.Path)
	}
	if rec.Body != "{}" {
		t.Fatalf("body = %q, want {}", rec.Body)
	}
}

func TestSteerPrompts(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"POST /api/v1/sessions/s1/prompts::steer": `{"code":0,"msg":"success","data":{"steered":true,"prompt_ids":["p1","p2"]},"request_id":"r6"}`,
	})
	c := New(srv.URL, "tok")
	if err := c.SteerPrompts(context.Background(), "s1", []string{"p1", "p2"}); err != nil {
		t.Fatalf("SteerPrompts: %v", err)
	}
	if rec.Method != http.MethodPost || rec.Path != "/api/v1/sessions/s1/prompts::steer" {
		t.Fatalf("request = %s %s", rec.Method, rec.Path)
	}
	if rec.Authorization != "Bearer tok" {
		t.Fatalf("Authorization = %q", rec.Authorization)
	}
	var body struct {
		PromptIDs []string `json:"prompt_ids"`
	}
	if err := json.Unmarshal([]byte(rec.Body), &body); err != nil || len(body.PromptIDs) != 2 || body.PromptIDs[0] != "p1" || body.PromptIDs[1] != "p2" {
		t.Fatalf("body = %s, err = %v", rec.Body, err)
	}
}

func TestSteerPromptsAPIError(t *testing.T) {
	srv, _ := restServer(t, map[string]string{
		"POST /api/v1/sessions/s1/prompts::steer": `{"code":40402,"msg":"one or more prompts are not pending","data":null,"request_id":"r7"}`,
	})
	c := New(srv.URL, "tok")
	err := c.SteerPrompts(context.Background(), "s1", []string{"p1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAPI) {
		t.Fatalf("errors.Is(err, ErrAPI) = false, err = %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 40402 {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}

func TestAPIError(t *testing.T) {
	srv, _ := restServer(t, map[string]string{
		"GET /api/v1/sessions/nope": `{"code":40401,"msg":"session nope does not exist","data":null,"request_id":"r5"}`,
	})
	c := New(srv.URL, "tok")
	_, err := c.GetSnapshot(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAPI) {
		t.Fatalf("errors.Is(err, ErrAPI) = false, err = %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(*APIError) = false, err = %v", err)
	}
	if apiErr.Code != 40401 || apiErr.Msg != "session nope does not exist" || apiErr.RequestID != "r5" {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}

func TestSessionConflictAPIError(t *testing.T) {
	tests := []struct {
		name     string
		response string
		code     int
		msg      string
		details  string
	}{
		{
			name:     "occupied",
			response: `{"code":40931,"msg":"session s1 is occupied","data":null,"request_id":"r8","details":{"holder":{"clientIdentity":"other"}}}`,
			code:     40931,
			msg:      "session s1 is occupied",
			details:  `{"holder":{"clientIdentity":"other"}}`,
		},
		{
			name:     "stale",
			response: `{"code":40932,"msg":"session s1 is stale","data":null,"request_id":"r9","details":{"revision":7}}`,
			code:     40932,
			msg:      "session s1 is stale",
			details:  `{"revision":7}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := restServer(t, map[string]string{
				"POST /api/v1/sessions/s1/prompts": tt.response,
			})
			c := New(srv.URL, "tok")
			_, err := c.PostPrompt(context.Background(), "s1", PromptRequest{Text: "hi"})
			if !errors.Is(err, ErrAPI) {
				t.Fatalf("errors.Is(err, ErrAPI) = false, err = %v", err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As(*APIError) = false, err = %v", err)
			}
			if apiErr.Code != tt.code || apiErr.Msg != tt.msg || string(apiErr.Details) != tt.details {
				t.Fatalf("apiErr = %+v, details = %s", apiErr, apiErr.Details)
			}
		})
	}
}

func TestNonEnvelopeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>bad gateway</html>")
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")
	err := c.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrAPI) {
		t.Fatalf("non-envelope must not map to APIError, err = %v", err)
	}
}

func TestDefaultHTTPClientHasTimeout(t *testing.T) {
	if defaultHTTPClient.Timeout != 30*time.Second {
		t.Fatalf("defaultHTTPClient timeout = %s", defaultHTTPClient.Timeout)
	}
}

func TestInjectedHTTPClientOverridesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{"ok":true}}`)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")
	c.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	if err := c.Healthz(context.Background()); err == nil {
		t.Fatal("expected timeout error from injected client")
	}
}

func TestDoRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")
	err := c.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionRestoreAndReload(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"POST /api/v1/sessions/s1:restore": `{"code":0,"data":{"id":"s1","workspace_id":"w1","busy":false,"archived":false,"metadata":{"cwd":"/work"},"agent_config":{"model":"k2"},"usage":{"input_tokens":0,"output_tokens":0,"cache_read_tokens":0,"cache_creation_tokens":0,"context_tokens":0}}}`,
		"POST /api/v1/sessions/s1:reload":  `{"code":0,"data":{"id":"s1","workspace_id":"w1","busy":false,"archived":false,"metadata":{"cwd":"/work"},"agent_config":{"model":"k2"},"usage":{"input_tokens":0,"output_tokens":0,"cache_read_tokens":0,"cache_creation_tokens":0,"context_tokens":0}}}`,
	})
	client := New(srv.URL, "")
	session, err := client.RestoreSession(context.Background(), "s1")
	if err != nil || session.ID != "s1" || session.Archived {
		t.Fatalf("RestoreSession = %+v, err = %v", session, err)
	}
	if rec.Path != "/api/v1/sessions/s1:restore" || rec.Body != "{}" {
		t.Fatalf("restore request = %s %s", rec.Path, rec.Body)
	}
	session, err = client.ReloadSession(context.Background(), "s1")
	if err != nil || session.ID != "s1" {
		t.Fatalf("ReloadSession = %+v, err = %v", session, err)
	}
	if rec.Path != "/api/v1/sessions/s1:reload" || rec.Body != "{}" {
		t.Fatalf("reload request = %s %s", rec.Path, rec.Body)
	}
}

func TestListMCPServers(t *testing.T) {
	srv, rec := restServer(t, map[string]string{
		"GET /api/v1/mcp/servers": `{"code":0,"data":{"servers":[{"id":"mcp-1","name":"mcp-1","transport":"stdio","status":"running","tool_count":3,"next_retry_at":123}]}}`,
	})
	servers, err := New(srv.URL, "").ListMCPServers(context.Background())
	if err != nil || len(servers) != 1 || servers[0].ID != "mcp-1" || servers[0].Name != "mcp-1" || servers[0].ToolCount != 3 || servers[0].NextRetryAt != 123 {
		t.Fatalf("ListMCPServers = %+v, err = %v", servers, err)
	}
	if rec.RawQuery != "" {
		t.Fatalf("query = %q, want empty", rec.RawQuery)
	}
}
