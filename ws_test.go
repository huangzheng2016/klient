package klient

import (
	"context"
	"encoding/json"
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

		serverHello := `{"type":"server_hello","timestamp":"2026-01-01T00:00:00Z","payload":{"ws_connection_id":"conn_1","protocol_version":2,"max_event_buffer_size":1000,"capabilities":{"event_batching":false,"compression":false}}}`
		if err := conn.Write(ctx, websocket.MessageText, []byte(serverHello)); err != nil {
			fail(fmt.Errorf("write server_hello: %w", err))
			return
		}

		_, data, err = conn.Read(ctx)
		if err != nil {
			fail(fmt.Errorf("read subscribe: %w", err))
			return
		}
		var sub wsSubscribe
		if err := json.Unmarshal(data, &sub); err != nil {
			fail(fmt.Errorf("parse subscribe: %w", err))
			return
		}
		ts.sub <- sub

		script(ctx, conn)
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func eventFrame(t *testing.T, seq int64, sessionID, payload string) []byte {
	t.Helper()
	var p struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	return []byte(fmt.Sprintf(
		`{"type":%q,"seq":%d,"epoch":"ep1","session_id":%q,"timestamp":"2026-01-01T00:00:00Z","payload":%s}`,
		p.Type, seq, sessionID, payload))
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

func TestSubscribeHandshakeAndEvents(t *testing.T) {
	frames := [][]byte{
		[]byte(`{"type":"ack","id":"1","code":0,"msg":"success","payload":{"accepted_subscriptions":[],"resync_required":[]}}`),
		[]byte(`{"type":"ping","timestamp":"2026-01-01T00:00:00Z","payload":{"nonce":"n1"}}`),
	}
	payloads := []string{
		`{"type":"turn.started","turnId":1,"origin":{"kind":"user"},"prompt":"hi","agentId":"main","sessionId":"s1"}`,
		`{"type":"assistant.delta","turnId":1,"delta":"Hello","agentId":"main","sessionId":"s1"}`,
		`{"type":"thinking.delta","turnId":1,"delta":"hmm","agentId":"main","sessionId":"s1"}`,
		`{"type":"tool.call.started","turnId":1,"toolCallId":"tc1","name":"Bash","args":{"command":"ls"},"description":"list","agentId":"main","sessionId":"s1"}`,
		`{"type":"tool.call.delta","turnId":1,"toolCallId":"tc1","argumentsPart":"{\"command\":","agentId":"main","sessionId":"s1"}`,
		`{"type":"tool.progress","turnId":1,"toolCallId":"tc1","update":{"kind":"stdout","text":"out","percent":50},"agentId":"main","sessionId":"s1"}`,
		`{"type":"tool.result","turnId":1,"toolCallId":"tc1","output":{"stdout":"ok"},"synthetic":true,"agentId":"main","sessionId":"s1"}`,
		`{"type":"turn.step.started","turnId":1,"step":2,"stepId":"st1","agentId":"main","sessionId":"s1"}`,
		`{"type":"turn.step.completed","turnId":1,"step":2,"stepId":"st1","usage":{"inputOther":10,"output":5,"inputCacheRead":3,"inputCacheCreation":2},"finishReason":"tool_calls","agentId":"main","sessionId":"s1"}`,
		`{"type":"agent.status.updated","model":"k2","contextTokens":100,"maxContextTokens":200000,"contextUsage":0.5,"planMode":true,"permission":"auto","phase":{"kind":"running","turnId":1,"step":2,"stepId":"st1","since":1},"usage":{"currentTurn":{"inputOther":1,"output":2,"inputCacheRead":3,"inputCacheCreation":4}},"agentId":"main","sessionId":"s1"}`,
		`{"type":"prompt.completed","promptId":"p9","finishedAt":"2026-01-01T00:00:01Z","reason":"completed","agentId":"main","sessionId":"s1"}`,
		`{"type":"prompt.aborted","promptId":"p8","abortedAt":"2026-01-01T00:00:02Z","agentId":"main","sessionId":"s1"}`,
		`{"type":"error","code":"provider.api_error","message":"boom","retryable":true,"agentId":"main","sessionId":"s1"}`,
		`{"type":"warning","message":"careful","code":"agents-md-oversized","agentId":"main","sessionId":"s1"}`,
		`{"type":"turn.ended","turnId":1,"reason":"completed","durationMs":100,"traceId":"trace-1","agentId":"main","sessionId":"s1"}`,
		`{"type":"event.session.work_changed","busy":false,"main_turn_active":false,"last_turn_reason":"completed","agentId":"main","sessionId":"s1"}`,
	}
	for i, p := range payloads {
		frames = append(frames, eventFrame(t, int64(i+1), "s1", p))
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

	c := New(ts.srv.URL, "tok")
	subscription, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Close()
	ch := subscription.Events

	if got := <-ts.auth; got != "Bearer tok" {
		t.Fatalf("ws Authorization = %q", got)
	}
	hello := <-ts.hello
	if hello.Type != "client_hello" || hello.ID != "1" || hello.Payload.ClientID != "klient-go" {
		t.Fatalf("client_hello = %+v", hello)
	}
	sub := <-ts.sub
	if sub.Type != "subscribe" || len(sub.Payload.SessionIDs) != 1 || sub.Payload.SessionIDs[0] != "s1" {
		t.Fatalf("subscribe = %+v", sub)
	}
	if sub.Payload.Cursors != nil {
		t.Fatalf("zero cursor must not send cursors, got %+v", sub.Payload.Cursors)
	}

	wantTypes := []string{
		TypeTurnStarted, TypeAssistantDelta, TypeThinkingDelta,
		TypeToolCallStarted, TypeToolCallDelta, TypeToolProgress, TypeToolResult,
		TypeTurnStepStarted, TypeTurnStepComplete, TypeAgentStatus,
		TypePromptCompleted, TypePromptAborted, TypeError, TypeWarning,
		TypeTurnEnded, TypeWorkChanged,
	}
	evs := make([]Event, 0, len(wantTypes))
	for range wantTypes {
		evs = append(evs, recvEvent(t, ch))
	}
	waitClosed(t, ch)

	for i, want := range wantTypes {
		if evs[i].Type != want {
			t.Fatalf("event %d type = %q, want %q", i, evs[i].Type, want)
		}
		if evs[i].Seq != int64(i+1) || evs[i].Epoch != "ep1" || evs[i].SessionID != "s1" {
			t.Fatalf("event %d envelope = seq %d epoch %q session %q",
				i, evs[i].Seq, evs[i].Epoch, evs[i].SessionID)
		}
	}

	decode := func(i int, v any) {
		t.Helper()
		if err := evs[i].Decode(v); err != nil {
			t.Fatalf("decode event %d (%s): %v", i, evs[i].Type, err)
		}
	}

	var turnStarted TurnStartedPayload
	decode(0, &turnStarted)
	if turnStarted.TurnID != 1 || turnStarted.Origin.Kind != "user" || turnStarted.Prompt != "hi" {
		t.Fatalf("turn.started = %+v", turnStarted)
	}

	var assistantDelta AssistantDeltaPayload
	decode(1, &assistantDelta)
	if assistantDelta.TurnID != 1 || assistantDelta.Delta != "Hello" {
		t.Fatalf("assistant.delta = %+v", assistantDelta)
	}

	var thinkingDelta ThinkingDeltaPayload
	decode(2, &thinkingDelta)
	if thinkingDelta.Delta != "hmm" {
		t.Fatalf("thinking.delta = %+v", thinkingDelta)
	}

	var toolStarted ToolCallStartedPayload
	decode(3, &toolStarted)
	if toolStarted.ToolCallID != "tc1" || toolStarted.Name != "Bash" ||
		string(toolStarted.Args) != `{"command":"ls"}` || toolStarted.Description != "list" {
		t.Fatalf("tool.call.started = %+v", toolStarted)
	}

	var toolDelta ToolCallDeltaPayload
	decode(4, &toolDelta)
	if toolDelta.ToolCallID != "tc1" || toolDelta.ArgumentsPart != `{"command":` {
		t.Fatalf("tool.call.delta = %+v", toolDelta)
	}

	var toolProgress ToolProgressPayload
	decode(5, &toolProgress)
	if toolProgress.Update.Kind != "stdout" || toolProgress.Update.Text != "out" ||
		toolProgress.Update.Percent != 50 {
		t.Fatalf("tool.progress = %+v", toolProgress)
	}

	var toolResult ToolResultPayload
	decode(6, &toolResult)
	if string(toolResult.Output) != `{"stdout":"ok"}` || toolResult.IsError || !toolResult.Synthetic {
		t.Fatalf("tool.result = %+v", toolResult)
	}

	var stepStarted TurnStepPayload
	decode(7, &stepStarted)
	if stepStarted.TurnID != 1 || stepStarted.Step != 2 || stepStarted.StepID != "st1" {
		t.Fatalf("turn.step.started = %+v", stepStarted)
	}

	var stepCompleted TurnStepCompletedPayload
	decode(8, &stepCompleted)
	if stepCompleted.Usage == nil || stepCompleted.Usage.InputOther != 10 ||
		stepCompleted.Usage.Output != 5 || stepCompleted.FinishReason != "tool_calls" {
		t.Fatalf("turn.step.completed = %+v", stepCompleted)
	}

	var agentStatus AgentStatusPayload
	decode(9, &agentStatus)
	if agentStatus.Model != "k2" || agentStatus.MaxContextTokens != 200000 ||
		agentStatus.ContextUsage != 0.5 || !agentStatus.PlanMode || agentStatus.Permission != "auto" {
		t.Fatalf("agent.status.updated = %+v", agentStatus)
	}
	if agentStatus.Phase == nil || agentStatus.Phase.Kind != "running" || agentStatus.Phase.TurnID != 1 {
		t.Fatalf("agent.status.updated.phase = %+v", agentStatus.Phase)
	}
	if agentStatus.Usage == nil || agentStatus.Usage.CurrentTurn == nil ||
		agentStatus.Usage.CurrentTurn.Output != 2 {
		t.Fatalf("agent.status.updated.usage = %+v", agentStatus.Usage)
	}

	var promptCompleted PromptCompletedPayload
	decode(10, &promptCompleted)
	if promptCompleted.PromptID != "p9" || promptCompleted.FinishedAt != "2026-01-01T00:00:01Z" ||
		promptCompleted.Reason != "completed" {
		t.Fatalf("prompt.completed = %+v", promptCompleted)
	}

	var promptAborted PromptAbortedPayload
	decode(11, &promptAborted)
	if promptAborted.PromptID != "p8" || promptAborted.AbortedAt != "2026-01-01T00:00:02Z" {
		t.Fatalf("prompt.aborted = %+v", promptAborted)
	}

	var errPayload ErrorPayload
	decode(12, &errPayload)
	if errPayload.Code != "provider.api_error" || errPayload.Message != "boom" || !errPayload.Retryable {
		t.Fatalf("error = %+v", errPayload)
	}

	var warning WarningPayload
	decode(13, &warning)
	if warning.Message != "careful" || warning.Code != "agents-md-oversized" {
		t.Fatalf("warning = %+v", warning)
	}

	var turnEnded TurnEndedPayload
	decode(14, &turnEnded)
	if turnEnded.TurnID != 1 || turnEnded.Reason != "completed" || turnEnded.DurationMs != 100 ||
		turnEnded.TraceID != "trace-1" {
		t.Fatalf("turn.ended = %+v", turnEnded)
	}

	var workChanged WorkChangedPayload
	decode(15, &workChanged)
	if workChanged.Busy || workChanged.MainTurnActive || workChanged.LastTurnReason != "completed" {
		t.Fatalf("work_changed = %+v", workChanged)
	}

	select {
	case err := <-ts.handlerErr:
		t.Fatalf("server handler: %v", err)
	default:
	}
}

func TestSubscribeWithCursor(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	c := New(ts.srv.URL, "")
	subscription, err := c.Subscribe(context.Background(), "s1", Cursor{Seq: 5, Epoch: "e9"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ch := subscription.Events
	sub := <-ts.sub
	cursor, ok := sub.Payload.Cursors["s1"]
	if !ok || cursor.Seq != 5 || cursor.Epoch != "e9" {
		t.Fatalf("subscribe cursors = %+v", sub.Payload.Cursors)
	}
	waitClosed(t, ch)
}

func TestResyncRequired(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		frames := [][]byte{
			eventFrame(t, 7, "s1", `{"type":"assistant.delta","turnId":1,"delta":"x","agentId":"main","sessionId":"s1"}`),
			[]byte(`{"type":"resync_required","timestamp":"2026-01-01T00:00:00Z","payload":{"session_id":"s1","reason":"epoch_changed","current_seq":9,"epoch":"e2"}}`),
		}
		_ = writeAll(ctx, conn, frames...)
		_, _, _ = conn.Read(ctx)
	})

	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{Seq: 7, Epoch: "e1"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()
	ch := sub.Events

	delta := recvEvent(t, ch)
	if delta.Type != TypeAssistantDelta {
		t.Fatalf("first event = %q", delta.Type)
	}
	resync := recvEvent(t, ch)
	if resync.Type != TypeResyncRequired {
		t.Fatalf("second event = %q", resync.Type)
	}
	if resync.SessionID != "s1" || resync.Seq != 9 || resync.Epoch != "e2" {
		t.Fatalf("resync event = %+v", resync)
	}
	var p struct {
		Reason string `json:"reason"`
	}
	if err := resync.Decode(&p); err != nil || p.Reason != "epoch_changed" {
		t.Fatalf("resync payload = %s, err = %v", resync.Payload, err)
	}
	waitClosed(t, ch)
}

func TestControlErrorFrame(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		frames := [][]byte{
			[]byte(`{"type":"error","timestamp":"2026-01-01T00:00:00Z","payload":{"code":50001,"msg":"fatal boom","fatal":true}}`),
			eventFrame(t, 1, "s1", `{"type":"assistant.delta","turnId":1,"delta":"x","agentId":"main","sessionId":"s1"}`),
		}
		_ = writeAll(ctx, conn, frames...)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ch := sub.Events
	errEv := recvEvent(t, ch)
	if errEv.Type != TypeProtocolError {
		t.Fatalf("first event = %q, want %q", errEv.Type, TypeProtocolError)
	}
	var p ProtocolErrorPayload
	if err := errEv.Decode(&p); err != nil {
		t.Fatalf("decode protocol error: %v", err)
	}
	if p.Code != 50001 || p.Msg != "fatal boom" || !p.Fatal {
		t.Fatalf("protocol error payload = %+v", p)
	}
	if ev := recvEvent(t, ch); ev.Type != TypeAssistantDelta {
		t.Fatalf("second event = %q", ev.Type)
	}
	waitClosed(t, ch)
}

func TestSubscriptionCloseStopsReadLoop(t *testing.T) {
	ts := newWSTestServer(t, func(ctx context.Context, conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})

	c := New(ts.srv.URL, "")
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
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

	c := New(ts.srv.URL, "")
	ctx, cancel := context.WithCancel(context.Background())
	sub, err := c.Subscribe(ctx, "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ch := sub.Events
	<-ts.sub
	cancel()
	waitClosed(t, ch)
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
		_ = writeAll(ctx, conn, eventFrame(t, 1, "s1", `{"type":"assistant.delta","turnId":1,"delta":"`+big+`","agentId":"main","sessionId":"s1"}`))
		_, _, _ = conn.Read(ctx)
	})

	c := New(ts.srv.URL, "")
	c.MaxFrameBytes = 1024
	sub, err := c.Subscribe(context.Background(), "s1", Cursor{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ch := sub.Events
	ev := recvEvent(t, ch)
	if ev.Type != TypeReadError {
		t.Fatalf("first event = %q, want %q", ev.Type, TypeReadError)
	}
	var p struct {
		Message string `json:"message"`
	}
	if err := ev.Decode(&p); err != nil || !strings.Contains(p.Message, "message too big") {
		t.Fatalf("read error payload = %s, err = %v", ev.Payload, err)
	}
	waitClosed(t, ch)
}
