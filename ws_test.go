package klient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type wsTestServer struct {
	srv        *httptest.Server
	auth       chan string
	hello      chan wsClientHello
	sub        chan wsSubscribe
	handlerErr chan error
}

func newWSTestServer(t *testing.T, script func(ctx context.Context, conn *websocket.Conn)) *wsTestServer {
	return newWSTestServerHandshake(t, "", nil, script)
}

func newWSTestServerHandshake(t *testing.T, ack string, preAck [][]byte, script func(ctx context.Context, conn *websocket.Conn)) *wsTestServer {
	t.Helper()
	ts := &wsTestServer{
		auth:       make(chan string, 1),
		hello:      make(chan wsClientHello, 1),
		sub:        make(chan wsSubscribe, 1),
		handlerErr: make(chan error, 1),
	}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.auth <- r.Header.Get("Authorization")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			ts.handlerErr <- fmt.Errorf("accept: %w", err)
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fail := func(err error) {
			select {
			case ts.handlerErr <- err:
			default:
			}
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			fail(fmt.Errorf("read client_hello: %w", err))
			return
		}
		var hello wsClientHello
		if err := json.Unmarshal(data, &hello); err != nil {
			fail(fmt.Errorf("parse client_hello: %w", err))
			return
		}
		ts.hello <- hello
		serverHello := `{"type":"server_hello","timestamp":"2026-01-01T00:00:00Z","payload":{"ws_connection_id":"conn_1","protocol_version":2,"capabilities":{"event_batching":false,"compression":false}}}`
		if err := conn.Write(ctx, websocket.MessageText, []byte(serverHello)); err != nil {
			fail(fmt.Errorf("write server_hello: %w", err))
			return
		}
		helloAck := fmt.Sprintf(`{"type":"ack","id":%q,"code":0,"msg":"success","payload":{}}`, hello.ID)
		if err := conn.Write(ctx, websocket.MessageText, []byte(helloAck)); err != nil {
			fail(fmt.Errorf("write client_hello ack: %w", err))
			return
		}

		_, data, err = conn.Read(ctx)
		if err != nil {
			fail(fmt.Errorf("read subscribe_v2: %w", err))
			return
		}
		var sub wsSubscribe
		if err := json.Unmarshal(data, &sub); err != nil {
			fail(fmt.Errorf("parse subscribe_v2: %w", err))
			return
		}
		ts.sub <- sub
		for _, frame := range preAck {
			if err := conn.Write(ctx, websocket.MessageText, frame); err != nil {
				fail(fmt.Errorf("write pre-ack frame: %w", err))
				return
			}
		}
		if ack != "skip" {
			if ack == "" {
				ack = fmt.Sprintf(`{"type":"ack","id":%q,"code":0,"msg":"success","payload":{"accepted":[%q],"not_found":[]}}`, sub.ID, sub.Payload.SessionID)
			}
			if err := conn.Write(ctx, websocket.MessageText, []byte(ack)); err != nil {
				fail(fmt.Errorf("write subscribe_v2 ack: %w", err))
				return
			}
		}
		script(ctx, conn)
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func transcriptReset(agentID string, seq int64, snapshot string) []byte {
	if snapshot == "" {
		snapshot = `{"items":[],"interactions":[],"attachments":[],"prompts":[]}`
	}
	return []byte(fmt.Sprintf(`{"type":"transcript.reset","session_id":"s1","timestamp":"2026-01-01T00:00:00Z","payload":{"type":"transcript.reset","agent_id":%q,"snapshot":%s,"has_more_older":false,"seq":%d}}`, agentID, snapshot, seq))
}

func transcriptOps(agentID string, seq int64, ops ...string) []byte {
	return []byte(fmt.Sprintf(`{"type":"transcript.ops","session_id":"s1","timestamp":"2026-01-01T00:00:00Z","payload":{"type":"transcript.ops","agent_id":%q,"ops":[%s],"seq":%d}}`, agentID, strings.Join(ops, ","), seq))
}

func writeAll(ctx context.Context, conn *websocket.Conn, frames ...[]byte) error {
	for _, f := range frames {
		if err := conn.Write(ctx, websocket.MessageText, f); err != nil {
			return err
		}
	}
	return nil
}

func recvEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed unexpectedly")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
		return Event{}
	}
}

func waitClosed(t *testing.T, ch <-chan Event) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("expected closed channel, got event %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func waitTranscriptSeq(t *testing.T, sub *Subscription, agentID string, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sub.Cursor().TranscriptSince[agentID] == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("cursor %s = %d, want %d", agentID, sub.Cursor().TranscriptSince[agentID], want)
}

func TestPingAnsweredWithPong(t *testing.T) {
	var ts *wsTestServer
	pong := make(chan map[string]any, 1)
	ts = newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		ping := `{"type":"ping","timestamp":"2026-01-01T00:00:00Z","payload":{"nonce":"n42"}}`
		if err := conn.Write(ctx, websocket.MessageText, []byte(ping)); err != nil {
			select {
			case ts.handlerErr <- err:
			default:
			}
			return
		}
		_, data, err := conn.Read(ctx)
		if err != nil {
			select {
			case ts.handlerErr <- fmt.Errorf("read pong: %w", err):
			default:
			}
			return
		}
		var frame map[string]any
		if json.Unmarshal(data, &frame) == nil {
			pong <- frame
		}
	})

	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	select {
	case frame := <-pong:
		if frame["type"] != "pong" {
			t.Fatalf("frame type = %v, want pong: %v", frame["type"], frame)
		}
		payload, _ := frame["payload"].(map[string]any)
		if payload["nonce"] != "n42" {
			t.Fatalf("pong nonce = %v, want n42", payload["nonce"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client did not answer ping with pong")
	}
}

func TestSubscribeV2HandshakeAndCursor(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	cursor := Cursor{TranscriptSince: map[string]int64{"main": 5, "agent-0": 9}}
	c := New(ts.srv.URL, "tok")
	subscription, err := c.Subscribe(context.Background(), "s1", cursor)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if got := <-ts.auth; got != "Bearer tok" {
		t.Fatalf("ws Authorization = %q", got)
	}
	hello := <-ts.hello
	if hello.Type != "client_hello" || hello.ID != "1" || hello.Payload.ClientID != "klient-go" {
		t.Fatalf("client_hello = %+v", hello)
	}
	sub := <-ts.sub
	if sub.Type != "subscribe_v2" || sub.Payload.SessionID != "s1" || sub.Payload.Transcript["*"] != "delta" {
		t.Fatalf("subscribe_v2 = %+v", sub)
	}
	if sub.Payload.TranscriptSince["main"] != 5 || sub.Payload.TranscriptSince["agent-0"] != 9 {
		t.Fatalf("transcript_since = %+v", sub.Payload.TranscriptSince)
	}
	waitClosed(t, subscription.Events)
}

func TestSubscribeAckError(t *testing.T) {
	ack := `{"type":"ack","id":"2","code":1,"msg":"invalid subscribe_v2 payload","payload":{}}`
	ts := newWSTestServerHandshake(t, ack, nil, func(ctx context.Context, conn *websocket.Conn) {})
	_, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err == nil || !errors.Is(err, ErrAPI) {
		t.Fatalf("Subscribe error = %v, want ErrAPI", err)
	}
	var subErr *SubscribeError
	if !errors.As(err, &subErr) || subErr.Code != 1 {
		t.Fatalf("SubscribeError = %+v, err = %v", subErr, err)
	}
}

func TestSubscribeNotFound(t *testing.T) {
	ack := `{"type":"ack","id":"2","code":0,"msg":"success","payload":{"accepted":[],"not_found":["s1"]}}`
	ts := newWSTestServerHandshake(t, ack, nil, func(ctx context.Context, conn *websocket.Conn) {})
	_, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err == nil || !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Subscribe error = %v, want ErrSessionNotFound", err)
	}
}

func TestSubscribeAckTimeout(t *testing.T) {
	ts := newWSTestServerHandshake(t, "skip", nil, func(ctx context.Context, conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})
	c := New(ts.srv.URL, "")
	c.DialTimeout = 50 * time.Millisecond
	_, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Subscribe error = %v, want deadline", err)
	}
}

func TestSubscribeBuffersResetBeforeAck(t *testing.T) {
	wrongAck := []byte(`{"type":"ack","id":"other","code":1,"msg":"ignored","payload":{}}`)
	snapshot := `{"items":[],"interactions":[],"attachments":[],"prompts":[{"promptId":"p1","status":"completed","createdAt":"2026-01-01T00:00:00Z","finishedAt":"2026-01-01T00:00:01Z"}]}`
	ts := newWSTestServerHandshake(t, "", [][]byte{wrongAck, transcriptReset("main", 4, snapshot)}, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	ev := recvEvent(t, sub.Events)
	if ev.Type != TypePromptCompleted || ev.Seq != 4 || ev.AgentID != "main" {
		t.Fatalf("buffered event = %+v", ev)
	}
	var p PromptCompletedPayload
	if err := ev.Decode(&p); err != nil || p.PromptID != "p1" || p.Reason != "completed" || p.AgentID != "main" {
		t.Fatalf("prompt payload = %+v, err = %v", p, err)
	}
	if got := sub.Cursor().TranscriptSince["main"]; got != 4 {
		t.Fatalf("cursor main = %d, want 4", got)
	}
	waitClosed(t, sub.Events)
}

func TestTranscriptMapsMainAndSubagentEvents(t *testing.T) {
	frames := [][]byte{
		transcriptReset("main", 0, ""),
		transcriptReset("agent-0", 0, ""),
		transcriptOps("main", 1,
			`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t1","triggerPromptId":"p1","ordinal":1,"state":"running","origin":{"kind":"user"},"prompt":"hi","startedAt":"2026-01-01T00:00:00Z"}}`,
			`{"op":"step.upsert","turnId":"t1","step":{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"running"}}`),
		transcriptOps("agent-0", 1,
			`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t90","ordinal":90,"state":"running","origin":{"kind":"task","taskId":"task-9"}}}`,
			`{"op":"frame.upsert","turnId":"t90","stepId":"t90.1","frame":{"kind":"text","frameId":"f90","role":"assistant","text":"child"}}`),
		transcriptOps("main", 2,
			`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":""}}`,
			`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":0,"text":"你"}`,
			`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":1,"text":"😀"}`),
		transcriptOps("main", 3,
			`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":3,"text":"好"}`,
			`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"thinking","frameId":"f2","text":""}}`,
			`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f2"},"offset":0,"text":"hmm"}`),
		transcriptOps("main", 4,
			`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"tool","frameId":"tool1","toolCallId":"tc1","name":"Bash","state":"running","input":{"command":"ls"}}}`),
		transcriptOps("main", 5,
			`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"tool","frameId":"tool1","toolCallId":"tc1","name":"Bash","state":"running","input":{"command":"ls"},"inputText":"{\"command\":\"ls\"}","progress":{"kind":"stdout","text":"out","percent":50}}}`),
		transcriptOps("main", 6,
			`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"tool","frameId":"tool1","toolCallId":"tc1","name":"Bash","state":"done","input":{"command":"ls"},"inputText":"{\"command\":\"ls\"}","output":{"stdout":"ok"},"agentRefs":[{"agentId":"agent-0","role":"child"}]}}`),
		transcriptOps("main", 7,
			`{"op":"step.upsert","turnId":"t1","step":{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"completed","usage":{"inputOther":10,"output":5,"inputCacheRead":3,"inputCacheCreation":2},"finishReason":"stop"}}`,
			`{"op":"prompt.upsert","prompt":{"promptId":"p1","status":"completed","createdAt":"2026-01-01T00:00:00Z","finishedAt":"2026-01-01T00:00:09Z"}}`,
			`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t1","triggerPromptId":"p1","ordinal":1,"state":"completed","origin":{"kind":"user"},"durationMs":100}}`),
		transcriptOps("agent-0", 2,
			`{"op":"prompt.upsert","prompt":{"promptId":"sub-p","status":"failed","createdAt":"2026-01-01T00:00:00Z","finishedAt":"2026-01-01T00:00:10Z"}}`),
	}
	var ts *wsTestServer
	ts = newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		if err := writeAll(ctx, conn, frames...); err != nil {
			select {
			case ts.handlerErr <- err:
			default:
			}
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	wantTypes := []string{
		TypeTurnStarted, TypeTurnStepStarted,
		TypeTurnStarted, TypeAssistantDelta,
		TypeAssistantDelta, TypeAssistantDelta,
		TypeAssistantDelta, TypeThinkingDelta,
		TypeToolCallStarted, TypeToolCallDelta, TypeToolProgress,
		TypeSubagentSpawned, TypeToolResult,
		TypeTurnStepComplete, TypePromptCompleted, TypeTurnEnded,
		TypePromptCompleted,
	}
	events := make([]Event, 0, len(wantTypes))
	for _, want := range wantTypes {
		ev := recvEvent(t, sub.Events)
		if ev.Type != want {
			t.Fatalf("event %d type = %q, want %q", len(events), ev.Type, want)
		}
		events = append(events, ev)
	}
	waitClosed(t, sub.Events)
	if events[2].AgentID != "agent-0" || events[2].Seq != 1 || events[3].AgentID != "agent-0" {
		t.Fatalf("subagent events = %+v %+v", events[2], events[3])
	}
	if events[4].Seq != 2 || events[5].Seq != 2 || events[6].Seq != 3 {
		t.Fatalf("main per-agent seq = %+v", events[4:7])
	}
	var assistant AssistantDeltaPayload
	if err := events[6].Decode(&assistant); err != nil || assistant.Delta != "好" || assistant.TurnID != "t1" || assistant.AgentID != "main" {
		t.Fatalf("assistant payload = %+v, err = %v", assistant, err)
	}
	var thinking ThinkingDeltaPayload
	if err := events[7].Decode(&thinking); err != nil || thinking.Delta != "hmm" {
		t.Fatalf("thinking payload = %+v, err = %v", thinking, err)
	}
	var toolStart ToolCallStartedPayload
	if err := events[8].Decode(&toolStart); err != nil || toolStart.ToolCallID != "tc1" || string(toolStart.Args) != `{"command":"ls"}` {
		t.Fatalf("tool start = %+v, err = %v", toolStart, err)
	}
	var toolDelta ToolCallDeltaPayload
	if err := events[9].Decode(&toolDelta); err != nil || toolDelta.ArgumentsPart != `{"command":"ls"}` {
		t.Fatalf("tool delta = %+v, err = %v", toolDelta, err)
	}
	var spawned SubagentSpawnedPayload
	if err := events[11].Decode(&spawned); err != nil || spawned.SubagentID != "agent-0" || spawned.ParentAgentID != "main" {
		t.Fatalf("subagent spawned = %+v, err = %v", spawned, err)
	}
	var result ToolResultPayload
	if err := events[12].Decode(&result); err != nil || string(result.Output) != `{"stdout":"ok"}` || result.IsError {
		t.Fatalf("tool result = %+v, err = %v", result, err)
	}
	var step TurnStepCompletedPayload
	if err := events[13].Decode(&step); err != nil || step.Usage == nil || step.Usage.InputOther != 10 || step.Usage.Output != 5 {
		t.Fatalf("step completed = %+v, err = %v", step, err)
	}
	var completed PromptCompletedPayload
	if err := events[14].Decode(&completed); err != nil || completed.PromptID != "p1" || completed.Reason != "completed" {
		t.Fatalf("main prompt completed = %+v, err = %v", completed, err)
	}
	var ended TurnEndedPayload
	if err := events[15].Decode(&ended); err != nil || ended.TurnID != "t1" || ended.Reason != "completed" || ended.PromptID != "p1" {
		t.Fatalf("turn ended = %+v, err = %v", ended, err)
	}
	if err := events[16].Decode(&completed); err != nil || completed.PromptID != "sub-p" || completed.Reason != "failed" || completed.AgentID != "agent-0" {
		t.Fatalf("sub prompt completed = %+v, err = %v", completed, err)
	}
	cursor := sub.Cursor().TranscriptSince
	if cursor["main"] != 7 || cursor["agent-0"] != 2 {
		t.Fatalf("cursor = %+v", cursor)
	}
}

func TestTranscriptResetSnapshotMapping(t *testing.T) {
	snapshot := `{"items":[{"kind":"turn","turnId":"t3","triggerPromptId":"p3","ordinal":3,"state":"completed","origin":{"kind":"user"},"durationMs":42,"steps":[{"kind":"step","stepId":"t3.1","turnId":"t3","ordinal":1,"state":"completed","usage":{"inputOther":2,"output":1,"inputCacheCreation":0},"frames":[{"kind":"text","frameId":"f3","role":"assistant","text":"done"}]}]}],"interactions":[],"attachments":[],"prompts":[{"promptId":"p3","status":"completed","createdAt":"2026-01-01T00:00:00Z","finishedAt":"2026-01-01T00:00:03Z"}]}`
	reset := transcriptReset("main", 9, snapshot)
	ts := newWSTestServerHandshake(t, "", [][]byte{reset, reset}, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	for i, want := range []string{TypeTurnStarted, TypeTurnStepStarted, TypeAssistantDelta, TypeTurnStepComplete, TypeTurnEnded, TypePromptCompleted} {
		if ev := recvEvent(t, sub.Events); ev.Type != want || ev.Seq != 9 || ev.EventIndex != i {
			t.Fatalf("reset event = %+v, want %s seq 9 index %d", ev, want, i)
		}
	}
	waitClosed(t, sub.Events)
}

func TestTranscriptReplayDeduplicatesAndAcceptsReset(t *testing.T) {
	turn := `{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t8","ordinal":8,"state":"running","origin":{"kind":"user"}}}`
	snapshot := `{"items":[],"interactions":[],"attachments":[],"prompts":[{"promptId":"p8","status":"completed","createdAt":"2026-01-01T00:00:00Z","finishedAt":"2026-01-01T00:00:08Z"}]}`
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptOps("main", 8, turn),
			transcriptOps("main", 8, turn),
			transcriptReset("main", 8, snapshot),
			transcriptReset("main", 8, snapshot),
			transcriptOps("main", 9,
				`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t9","ordinal":9,"state":"running","origin":{"kind":"user"}}}`,
				`{"op":"frame.upsert","turnId":"t9","stepId":"t9.1","frame":{"kind":"text","frameId":"f9","role":"assistant","text":"next"}}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	cursor := Cursor{TranscriptSince: map[string]int64{"main": 7}}
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", cursor)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	for _, want := range []string{TypeTurnStarted, TypePromptCompleted, TypeTurnStarted, TypeAssistantDelta} {
		if ev := recvEvent(t, sub.Events); ev.Type != want {
			t.Fatalf("event = %+v, want %s", ev, want)
		}
	}
	waitClosed(t, sub.Events)
}

func TestTranscriptSeqGapSurfaced(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageText, transcriptOps("main", 4,
			`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t4","ordinal":4,"state":"running","origin":{"kind":"user"}}}`))
		_, _, _ = conn.Read(ctx)
	})
	cursor := Cursor{TranscriptSince: map[string]int64{"main": 2}}
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", cursor)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	ev := recvEvent(t, sub.Events)
	if ev.Type != TypeReadError || !strings.Contains(string(ev.Payload), "expected=3 got=4") {
		t.Fatalf("gap event = %+v %s", ev, ev.Payload)
	}
	waitClosed(t, sub.Events)
}

func TestTranscriptAppendGapSurfaced(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1, `{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":"a"}}`),
			transcriptOps("main", 2, `{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":2,"text":"b"}`))
		_, _, _ = conn.Read(ctx)
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	if ev := recvEvent(t, sub.Events); ev.Type != TypeAssistantDelta {
		t.Fatalf("first event = %+v", ev)
	}
	ev := recvEvent(t, sub.Events)
	if ev.Type != TypeReadError || !strings.Contains(string(ev.Payload), "append offset beyond local text") {
		t.Fatalf("append gap = %+v %s", ev, ev.Payload)
	}
	waitClosed(t, sub.Events)
}

func TestReconnectPreservesTranscriptStateWithStaleCursor(t *testing.T) {
	first := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1,
				`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":""}}`,
				`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":0,"text":"Hel"}`))
		_, _, _ = conn.Read(ctx)
	})
	c := New(first.srv.URL, "")
	sub1, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	if ev := recvEvent(t, sub1.Events); ev.Type != TypeAssistantDelta {
		t.Fatalf("first delta = %+v", ev)
	}
	cursor := sub1.Cursor()
	cursor.TranscriptSince["main"] = 0
	sub1.Close()
	second := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptOps("main", 1,
				`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":""}}`,
				`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":0,"text":"Hel"}`),
			transcriptOps("main", 2,
				`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":3,"text":"lo"}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	c.BaseURL = second.srv.URL
	sub2, err := c.Subscribe(context.Background(), "s1", cursor)
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}
	defer sub2.Close()
	sub := <-second.sub
	if sub.Payload.TranscriptSince["main"] != 0 {
		t.Fatalf("reconnect transcript_since = %+v", sub.Payload.TranscriptSince)
	}
	ev := recvEvent(t, sub2.Events)
	var p AssistantDeltaPayload
	if ev.Type != TypeAssistantDelta || ev.Decode(&p) != nil || p.Delta != "lo" {
		t.Fatalf("reconnect delta = %+v payload=%+v", ev, p)
	}
	waitClosed(t, sub2.Events)
}

func TestAgentUsageTaskAndLifecycleFrames(t *testing.T) {
	frames := [][]byte{
		[]byte(`{"type":"agent","session_id":"s1","agent_id":"main","event_created_at":"2026-01-01T00:00:00Z","status":"running","model":"k2","thinking":"high","permission_mode":"yolo","plan_mode":true,"swarm_mode":false,"tower_mode":false}`),
		[]byte(`{"type":"usage","session_id":"s1","agent_id":"main","event_created_at":"2026-01-01T00:00:00Z","context_tokens":100,"max_context_tokens":200000}`),
		[]byte(`{"type":"task.list","session_id":"s1","agent_id":"main","tasks":[{"task_id":"task-1","kind":"shell","status":"running","detached":true,"command":"sleep 1"}]}`),
		[]byte(`{"type":"task","session_id":"s1","agent_id":"main","event_created_at":"2026-01-01T00:00:00Z","task_id":"task-2","kind":"subagent","status":"running","detached":true,"child_agent_id":"agent-0","parent_tool_call_id":"tc2","model":"k2"}`),
		[]byte(`{"type":"task","session_id":"s1","agent_id":"main","event_created_at":"2026-01-01T00:00:01Z","task_id":"task-2","kind":"subagent","status":"ended","outcome":"completed","detached":true,"child_agent_id":"agent-0","ended_at":"2026-01-01T00:00:01Z","result_summary":"done"}`),
		[]byte(`{"type":"task.output","session_id":"s1","agent_id":"main","task_id":"task-2","offset":4,"text":"tail"}`),
		[]byte(`{"type":"event.session.work_changed","session_id":"s1","timestamp":"2026-01-01T00:00:02Z","payload":{"type":"event.session.work_changed","busy":false,"main_turn_active":false,"last_turn_reason":"completed","agentId":"main","sessionId":"s1"}}`),
	}
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn, frames...)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	agent := recvEvent(t, sub.Events)
	var agentPayload AgentStatusPayload
	if agent.Type != TypeAgentStatus || agent.Decode(&agentPayload) != nil || agentPayload.Model != "k2" || agentPayload.ThinkingEffort != "high" || !agentPayload.PlanMode {
		t.Fatalf("agent event = %+v payload=%+v", agent, agentPayload)
	}
	usage := recvEvent(t, sub.Events)
	var usagePayload UsageContextPayload
	if usage.Type != TypeUsage || usage.Decode(&usagePayload) != nil || usagePayload.ContextTokens != 100 || usagePayload.MaxContextTokens != 200000 {
		t.Fatalf("usage event = %+v payload=%+v", usage, usagePayload)
	}
	list := recvEvent(t, sub.Events)
	var listPayload TaskListPayload
	if list.Type != TypeTaskList || list.Decode(&listPayload) != nil || len(listPayload.Tasks) != 1 || listPayload.Tasks[0].ID != "task-1" || !listPayload.Tasks[0].Detached {
		t.Fatalf("task list = %+v payload=%+v", list, listPayload)
	}
	started := recvEvent(t, sub.Events)
	var startedPayload TaskEventPayload
	if started.Type != TypeBackgroundTaskStarted || started.Decode(&startedPayload) != nil || startedPayload.Info.TaskID != "task-2" || startedPayload.Info.ChildAgentID != "agent-0" || !startedPayload.Info.Detached {
		t.Fatalf("task started = %+v payload=%+v", started, startedPayload)
	}
	terminated := recvEvent(t, sub.Events)
	var terminatedPayload TaskEventPayload
	if terminated.Type != TypeBackgroundTaskTerminated || terminated.Decode(&terminatedPayload) != nil || terminatedPayload.Info.Status != "completed" || terminatedPayload.Info.ResultSummary != "done" {
		t.Fatalf("task terminated = %+v payload=%+v", terminated, terminatedPayload)
	}
	output := recvEvent(t, sub.Events)
	var outputPayload TaskOutputPayload
	if output.Type != TypeTaskOutput || output.Decode(&outputPayload) != nil || outputPayload.Offset != 4 || outputPayload.Text != "tail" {
		t.Fatalf("task output = %+v payload=%+v", output, outputPayload)
	}
	work := recvEvent(t, sub.Events)
	var workPayload WorkChangedPayload
	if work.Type != TypeWorkChanged || work.AgentID != "main" || work.Decode(&workPayload) != nil || workPayload.LastTurnReason != "completed" {
		t.Fatalf("work changed = %+v payload=%+v", work, workPayload)
	}
	waitClosed(t, sub.Events)
}

func TestControlErrorFrame(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			[]byte(`{"type":"error","timestamp":"2026-01-01T00:00:00Z","payload":{"code":50001,"msg":"recoverable boom","fatal":false}}`),
			[]byte(`{"type":"event.session.work_changed","session_id":"s1","timestamp":"2026-01-01T00:00:00Z","payload":{"busy":false,"main_turn_active":false,"agentId":"main","sessionId":"s1"}}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	errEv := recvEvent(t, sub.Events)
	var p ProtocolErrorPayload
	if errEv.Type != TypeProtocolError || errEv.Decode(&p) != nil || p.Code != 50001 || p.Fatal {
		t.Fatalf("protocol error = %+v payload=%+v", errEv, p)
	}
	if ev := recvEvent(t, sub.Events); ev.Type != TypeWorkChanged {
		t.Fatalf("event after protocol error = %+v", ev)
	}
	waitClosed(t, sub.Events)
}

func TestSubscriptionCloseStopsReadLoop(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	<-ts.sub
	sub.Close()
	waitClosed(t, sub.Events)
	sub.Close()
}

func TestSubscribeCtxCancel(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})
	ctx, cancel := context.WithCancel(context.Background())
	sub, err := New(ts.srv.URL, "").Subscribe(ctx, "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	<-ts.sub
	cancel()
	waitClosed(t, sub.Events)
}

func TestSubscribeDialError(t *testing.T) {
	c := New("http://127.0.0.1:1", "")
	c.DialTimeout = time.Second
	if _, err := c.Subscribe(context.Background(), "s1", Cursor{}); err == nil {
		t.Fatal("expected dial error")
	}
}

func TestReadErrorSurfaced(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		big := strings.Repeat("x", 4096)
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1, `{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":"`+big+`"}}`))
		_, _, _ = conn.Read(ctx)
	})
	c := New(ts.srv.URL, "")
	c.MaxFrameBytes = 1024
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ev := recvEvent(t, sub.Events)
	if ev.Type != TypeReadError || !strings.Contains(string(ev.Payload), "message too big") {
		t.Fatalf("read error = %+v %s", ev, ev.Payload)
	}
	waitClosed(t, sub.Events)
}

func TestSnapshotCursorSeedsAppendBaseline(t *testing.T) {
	seq := int64(1)
	snapshot := &Snapshot{
		AgentID: "main",
		Seq:     &seq,
		Items: []TranscriptItem{{
			Kind: "turn", TurnID: "t1", State: "running",
			Steps: []TranscriptStep{{
				Kind: "step", StepID: "t1.1", TurnID: "t1", State: "running",
				Frames: []TranscriptFrame{{Kind: "text", FrameID: "f1", Role: "assistant", Text: "Hel"}},
			}},
		}},
	}
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageText, transcriptOps("main", 2,
			`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":3,"text":"lo"}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", snapshot.Cursor())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	ev := recvEvent(t, sub.Events)
	var p AssistantDeltaPayload
	if ev.Type != TypeAssistantDelta || ev.Decode(&p) != nil || p.Delta != "lo" {
		t.Fatalf("event = %+v payload = %+v", ev, p)
	}
	waitClosed(t, sub.Events)
}

func TestTranscriptResetWithinOpsUsesNewState(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1,
				`{"op":"reset","agentId":"main","snapshot":{"items":[],"interactions":[],"attachments":[],"prompts":[]}}`,
				`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t1","ordinal":1,"state":"running","origin":{"kind":"user"}}}`,
				`{"op":"step.upsert","turnId":"t1","step":{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"running"}}`,
				`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":""}}`,
				`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":0,"text":"A"}`),
			transcriptOps("main", 2,
				`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":1,"text":"B"}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	for _, want := range []string{TypeTurnStarted, TypeTurnStepStarted, TypeAssistantDelta, TypeAssistantDelta} {
		if ev := recvEvent(t, sub.Events); ev.Type != want {
			t.Fatalf("event = %+v, want %s", ev, want)
		}
	}
	waitClosed(t, sub.Events)
	if got := sub.Cursor().TranscriptSince["main"]; got != 2 {
		t.Fatalf("cursor main = %d, want 2", got)
	}
}

func TestConcurrentSessionSubscriptionRejected(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})
	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	defer sub.Close()
	stale := Cursor{TranscriptSince: map[string]int64{"main": 0}}
	if _, err := c.Subscribe(context.Background(), "s1", stale); !errors.Is(err, ErrSessionSubscribed) {
		t.Fatalf("second Subscribe error = %v, want ErrSessionSubscribed", err)
	}
}

func TestPartiallyConsumedBatchReplaysOnReconnect(t *testing.T) {
	batch := transcriptOps("main", 1,
		`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":"A"}}`,
		`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f2","role":"assistant","text":"B"}}`)
	first := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn, transcriptReset("main", 0, ""), batch)
		_, _, _ = conn.Read(ctx)
	})
	c := New(first.srv.URL, "")
	sub1, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	firstEvent := recvEvent(t, sub1.Events)
	if firstEvent.Type != TypeAssistantDelta || firstEvent.AgentID != "main" || firstEvent.Seq != 1 || firstEvent.EventIndex != 0 {
		t.Fatalf("first event = %+v", firstEvent)
	}
	cursor := sub1.Cursor()
	if got := cursor.TranscriptSince["main"]; got != 0 {
		t.Fatalf("cursor before batch commit = %d, want 0", got)
	}
	sub1.Close()
	second := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageText, batch)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	c.BaseURL = second.srv.URL
	sub2, err := c.Subscribe(context.Background(), "s1", cursor)
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}
	defer sub2.Close()
	for i, want := range []string{"A", "B"} {
		ev := recvEvent(t, sub2.Events)
		var p AssistantDeltaPayload
		if ev.Type != TypeAssistantDelta || ev.Decode(&p) != nil || p.Delta != want ||
			ev.AgentID != "main" || ev.Seq != 1 || ev.EventIndex != i {
			t.Fatalf("replayed event %d = %+v payload = %+v", i, ev, p)
		}
	}
	waitClosed(t, sub2.Events)
}

func TestCloseSessionReleasesCachedState(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1, `{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":"cached"}}`))
		_, _, _ = conn.Read(ctx)
	})
	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = recvEvent(t, sub.Events)
	sub.Close()
	c.streamMu.Lock()
	cached := c.streams["s1"] != nil
	c.streamMu.Unlock()
	if !cached {
		t.Fatal("session state was not cached before CloseSession")
	}
	c.CloseSession("s1")
	c.streamMu.Lock()
	released := c.streams["s1"] == nil
	c.streamMu.Unlock()
	if !released {
		t.Fatal("session state retained after CloseSession")
	}
}

func TestCloseSessionCancelsActiveSubscription(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})
	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	c.CloseSession("s1")
	waitClosed(t, sub.Events)
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.streams["s1"] != nil || c.active["s1"] != nil {
		t.Fatalf("session retained after CloseSession: streams=%v active=%v", c.streams, c.active)
	}
	if _, ok := c.active["s1"]; ok {
		t.Fatal("active session retained after CloseSession")
	}
}

func TestTranscriptSplitSurrogatePairs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     string
		role     string
		wantType string
	}{
		{name: "assistant", kind: "text", role: "assistant", wantType: TypeAssistantDelta},
		{name: "thinking", kind: "thinking", wantType: TypeThinkingDelta},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame := `{"kind":` + fmt.Sprintf("%q", tc.kind) + `,"frameId":"f1","text":""`
			if tc.role != "" {
				frame += `,"role":` + fmt.Sprintf("%q", tc.role)
			}
			frame += `}`
			ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
				_ = writeAll(ctx, conn,
					transcriptReset("main", 0, ""),
					transcriptOps("main", 1,
						`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":`+frame+`}`,
						`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":0,"text":"\ud83d"}`),
					transcriptOps("main", 2,
						`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":1,"text":"\ude00"}`),
					transcriptOps("main", 3,
						`{"op":"append","target":{"type":"frame","turnId":"t1","stepId":"t1.1","frameId":"f1"},"offset":2,"text":"!"}`))
				_ = conn.Close(websocket.StatusNormalClosure, "")
			})
			sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
			if err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			defer sub.Close()
			first := recvEvent(t, sub.Events)
			if first.Type != tc.wantType || strings.Contains(string(first.Payload), "ufffd") {
				t.Fatalf("split surrogate event = %+v %s", first, first.Payload)
			}
			var delta struct {
				Delta string `json:"delta"`
			}
			if err := first.Decode(&delta); err != nil || delta.Delta != "😀" {
				t.Fatalf("delta = %q, err = %v, payload = %s", delta.Delta, err, first.Payload)
			}
			second := recvEvent(t, sub.Events)
			if err := second.Decode(&delta); err != nil || delta.Delta != "!" {
				t.Fatalf("trailing delta = %q, err = %v", delta.Delta, err)
			}
			waitClosed(t, sub.Events)
		})
	}
}

func TestReconnectResetMapsOnlyNewContent(t *testing.T) {
	first := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1,
				`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t1","ordinal":1,"state":"running","origin":{"kind":"user"}}}`,
				`{"op":"step.upsert","turnId":"t1","step":{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"running"}}`,
				`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"old","role":"assistant","text":"old"}}`))
		_, _, _ = conn.Read(ctx)
	})
	c := New(first.srv.URL, "")
	sub1, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	for range 3 {
		_ = recvEvent(t, sub1.Events)
	}
	waitTranscriptSeq(t, sub1, "main", 1)
	sub1.Close()
	snapshot := `{"items":[{"kind":"turn","turnId":"t1","ordinal":1,"state":"running","origin":{"kind":"user"},"steps":[{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"running","frames":[{"kind":"text","frameId":"old","role":"assistant","text":"old"},{"kind":"text","frameId":"new","role":"assistant","text":"new"}]}]}],"interactions":[],"attachments":[],"prompts":[]}`
	second := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		reset := transcriptReset("main", 1, snapshot)
		_ = writeAll(ctx, conn, reset, reset)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	c.BaseURL = second.srv.URL
	sub2, err := c.Subscribe(context.Background(), "s1", sub1.Cursor())
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}
	defer sub2.Close()
	ev := recvEvent(t, sub2.Events)
	var p AssistantDeltaPayload
	if ev.Type != TypeAssistantDelta || ev.Decode(&p) != nil || p.Delta != "new" || ev.EventIndex != 0 || ev.Seq != 1 {
		t.Fatalf("reset delta = %+v payload = %+v", ev, p)
	}
	waitClosed(t, sub2.Events)
}

func TestOperationResetKeepsUndoSemantics(t *testing.T) {
	snapshot := `{"items":[{"kind":"turn","turnId":"t1","ordinal":1,"state":"running","origin":{"kind":"user"},"steps":[{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"running","frames":[{"kind":"text","frameId":"f1","role":"assistant","text":"old"}]}]}],"interactions":[],"attachments":[],"prompts":[]}`
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = writeAll(ctx, conn,
			transcriptReset("main", 0, ""),
			transcriptOps("main", 1,
				`{"op":"turn.upsert","turn":{"kind":"turn","turnId":"t1","ordinal":1,"state":"running","origin":{"kind":"user"}}}`,
				`{"op":"step.upsert","turnId":"t1","step":{"kind":"step","stepId":"t1.1","turnId":"t1","ordinal":1,"state":"running"}}`,
				`{"op":"frame.upsert","turnId":"t1","stepId":"t1.1","frame":{"kind":"text","frameId":"f1","role":"assistant","text":"old"}}`),
			transcriptOps("main", 2,
				`{"op":"reset","agentId":"main","snapshot":`+snapshot+`}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	sub, err := New(ts.srv.URL, "").Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	for i, want := range []string{TypeTurnStarted, TypeTurnStepStarted, TypeAssistantDelta, TypeTurnStarted, TypeTurnStepStarted, TypeAssistantDelta} {
		ev := recvEvent(t, sub.Events)
		if ev.Type != want {
			t.Fatalf("event %d = %+v, want %s", i, ev, want)
		}
		if i >= 3 && ev.EventIndex != i-3 {
			t.Fatalf("undo event %d index = %d, want %d", i, ev.EventIndex, i-3)
		}
	}
	waitClosed(t, sub.Events)
}

func TestSnapshotResetKeyNormalizesHasMoreOlder(t *testing.T) {
	seq := int64(4)
	snapshot := &Snapshot{AgentID: "main", Seq: &seq, HasMore: true}
	mapper := newTranscriptEventMapper("s1", snapshot.Cursor())
	want := mapper.resetKeys["main"]
	events, err := mapper.applyReset(transcriptResetPayload{
		AgentID: "main", Snapshot: transcriptSnapshot{}, HasMoreOlder: true, Seq: &seq,
	})
	if err != nil {
		t.Fatalf("applyReset: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
	if got := mapper.resetKeys["main"]; got != want {
		t.Fatalf("reset key changed across equivalent HasMore representations: before %q after %q", want, got)
	}
}
