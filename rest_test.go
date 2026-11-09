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
		"GET /api/v1/sessions/s1/snapshot": `{"code":0,"msg":"success","data":{"as_of_seq":42,"epoch":"ep1","session":null},"request_id":"r2"}`,
	})
	c := New(srv.URL, "")
	snap, err := c.GetSnapshot(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.AsOfSeq != 42 || snap.Epoch != "ep1" {
		t.Fatalf("snapshot = %+v", snap)
	}
	if cur := snap.Cursor(); cur.Seq != 42 || cur.Epoch != "ep1" {
		t.Fatalf("cursor = %+v", cur)
	}
	if rec.Authorization != "" {
		t.Fatalf("Authorization = %q, want empty", rec.Authorization)
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
	var recs []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		recs = append(recs, recordedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			ContentType:   r.Header.Get("Content-Type"),
			Body:          string(body),
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{"steered":true,"prompt_ids":["p1"]},"request_id":"r6"}`)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "tok")
	if err := c.SteerPrompts(context.Background(), "s1", []string{"p1", "p2"}); err != nil {
		t.Fatalf("SteerPrompts: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("requests = %d, want 2 (one per prompt id)", len(recs))
	}
	for i, want := range []string{"/api/v1/sessions/s1/prompts/p1:steer", "/api/v1/sessions/s1/prompts/p2:steer"} {
		if recs[i].Method != http.MethodPost || recs[i].Path != want {
			t.Fatalf("request[%d] = %s %s, want POST %s", i, recs[i].Method, recs[i].Path, want)
		}
		if recs[i].Authorization != "Bearer tok" {
			t.Fatalf("request[%d] Authorization = %q", i, recs[i].Authorization)
		}
		if recs[i].Body != "{}" {
			t.Fatalf("request[%d] body = %q, want {}", i, recs[i].Body)
		}
	}
}

func TestSteerPromptsAPIError(t *testing.T) {
	srv, _ := restServer(t, map[string]string{
		"POST /api/v1/sessions/s1/prompts/p1:steer": `{"code":40402,"msg":"one or more prompts are not pending","data":null,"request_id":"r7"}`,
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
		"GET /api/v1/sessions/nope/snapshot": `{"code":40401,"msg":"session nope does not exist","data":null,"request_id":"r5"}`,
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

func TestLifecycleSnapshotAndTaskContract(t *testing.T) {
	srv, _ := restServer(t, map[string]string{
		"GET /api/v1/sessions/s1/snapshot": `{"code":0,"data":{"as_of_seq":17,"epoch":"ep2","session":{"busy":true,"main_turn_active":true},"in_flight_turn":{"turn_id":4,"current_prompt_id":"p1","assistant_text":"","thinking_text":"","running_tools":[]},"messages":{"items":[{"metadata":{"origin":{"kind":"task","taskId":"task-1"}}}],"has_more":false}}}`,
		"GET /api/v1/sessions/s1/tasks":    `{"code":0,"data":{"items":[{"id":"task-1","session_id":"s1","kind":"bash","description":"test","status":"completed","created_at":"2026-10-30T00:00:00Z","completed_at":"2026-10-30T00:01:00Z","run_in_background":true}]}}`,
	})
	client := New(srv.URL, "")
	snapshot, err := client.GetSnapshot(context.Background(), "s1")
	if err != nil || !snapshot.Session.Busy || !snapshot.Session.MainTurnActive || snapshot.InFlightTurn == nil || snapshot.InFlightTurn.TurnID != 4 || snapshot.Messages.Items[0].Metadata.Origin.TaskID != "task-1" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	tasks, err := client.ListTasks(context.Background(), "s1")
	if err != nil || len(tasks) != 1 || tasks[0].ID != "task-1" || tasks[0].Status != "completed" || !tasks[0].RunInBackground || tasks[0].CompletedAt != "2026-10-30T00:01:00Z" {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
}
