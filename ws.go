package klient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

type wsClientHello struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload struct {
		ClientID string `json:"client_id"`
	} `json:"payload"`
}

type wsSubscribe struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload struct {
		SessionID       string            `json:"session_id"`
		Transcript      map[string]string `json:"transcript"`
		TranscriptSince map[string]int64  `json:"transcript_since,omitempty"`
	} `json:"payload"`
}

type wsFrame struct {
	Type           string          `json:"type"`
	ID             string          `json:"id"`
	SessionID      string          `json:"session_id"`
	AgentID        string          `json:"agent_id"`
	EventCreatedAt string          `json:"event_created_at"`
	Payload        json.RawMessage `json:"payload"`
	Code           int             `json:"code"`
	Msg            string          `json:"msg"`
	raw            json.RawMessage
}

type wsAck struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Payload struct {
		Accepted []string `json:"accepted"`
		NotFound []string `json:"not_found"`
	} `json:"payload"`
}

var ErrSessionNotFound = errors.New("klient: session not found")

var ErrSessionSubscribed = errors.New("klient: session already subscribed")

type SubscribeError struct {
	Op       string
	Code     int
	Msg      string
	Accepted []string
	NotFound []string
}

func (e *SubscribeError) Error() string {
	op := e.Op
	if op == "" {
		op = "subscribe_v2"
	}
	if len(e.NotFound) > 0 {
		return fmt.Sprintf("klient: %s session not found: %s", op, strings.Join(e.NotFound, ","))
	}
	return fmt.Sprintf("klient: %s ack code=%d msg=%q", op, e.Code, e.Msg)
}

func (e *SubscribeError) Unwrap() error {
	if len(e.NotFound) > 0 {
		return ErrSessionNotFound
	}
	return ErrAPI
}

type Subscription struct {
	Events <-chan Event

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
	mu     sync.RWMutex
	cursor Cursor
}

func (s *Subscription) Close() {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
	if s.done != nil {
		<-s.done
	}
}

func (s *Subscription) Cursor() Cursor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cursor.clone()
}

func (s *Subscription) setCursor(cursor Cursor) {
	s.mu.Lock()
	s.cursor = cursor
	s.mu.Unlock()
}

func (c *Client) subscribe(ctx context.Context, sessionID string, cursor Cursor) (*Subscription, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("klient: sessionID must not be empty")
	}
	wsURL, err := c.wsURL()
	if err != nil {
		return nil, err
	}
	mapper, err := c.takeTranscriptMapper(sessionID, cursor)
	if err != nil {
		return nil, err
	}
	keepMapper := false
	defer func() {
		if !keepMapper {
			c.putTranscriptMapper(sessionID, mapper)
		}
	}()

	handshakeCtx := ctx
	cancel := func() {}
	if c.DialTimeout > 0 {
		handshakeCtx, cancel = context.WithTimeout(ctx, c.DialTimeout)
	}
	defer cancel()

	header := http.Header{}
	if c.Token != "" {
		header.Set("Authorization", "Bearer "+c.Token)
	}
	conn, _, err := websocket.Dial(handshakeCtx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, fmt.Errorf("klient: ws dial: %w", err)
	}
	limit := c.MaxFrameBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	conn.SetReadLimit(limit)
	closeWithError := func(err error) (*Subscription, error) {
		conn.CloseNow()
		return nil, err
	}

	var hello wsClientHello
	hello.Type = "client_hello"
	hello.ID = "1"
	hello.Payload.ClientID = "klient-go"
	if err := writeFrame(handshakeCtx, conn, hello); err != nil {
		return closeWithError(err)
	}

	pending := make([]wsFrame, 0)
	serverHello := false
	helloAck := false
	for !serverHello || !helloAck {
		frame, err := readWSFrame(handshakeCtx, conn)
		if err != nil {
			return closeWithError(fmt.Errorf("klient: ws wait client_hello ack: %w", err))
		}
		switch frame.Type {
		case "server_hello":
			var payload struct {
				ProtocolVersion int `json:"protocol_version"`
			}
			if err := json.Unmarshal(frame.Payload, &payload); err != nil {
				return closeWithError(fmt.Errorf("klient: decode server_hello: %w", err))
			}
			if payload.ProtocolVersion != 2 {
				return closeWithError(fmt.Errorf("klient: unsupported WebSocket protocol version %d", payload.ProtocolVersion))
			}
			serverHello = true
		case "ack":
			if frame.ID != hello.ID {
				continue
			}
			if frame.Code != 0 {
				return closeWithError(&SubscribeError{Op: "client_hello", Code: frame.Code, Msg: frame.Msg})
			}
			helloAck = true
		case "ping":
			if err := answerPing(handshakeCtx, conn, frame.Payload); err != nil {
				return closeWithError(err)
			}
		case "error":
			return closeWithError(handshakeProtocolError(frame.Payload))
		default:
			pending = append(pending, frame)
		}
	}

	var sub wsSubscribe
	sub.Type = "subscribe_v2"
	sub.ID = "2"
	sub.Payload.SessionID = sessionID
	sub.Payload.Transcript = map[string]string{"*": "delta"}
	if len(cursor.TranscriptSince) > 0 {
		sub.Payload.TranscriptSince = cursor.clone().TranscriptSince
	}
	if err := writeFrame(handshakeCtx, conn, sub); err != nil {
		return closeWithError(err)
	}

	for {
		frame, err := readWSFrame(handshakeCtx, conn)
		if err != nil {
			return closeWithError(fmt.Errorf("klient: ws wait subscribe_v2 ack: %w", err))
		}
		switch frame.Type {
		case "ack":
			if frame.ID != sub.ID {
				continue
			}
			var ack wsAck
			if err := json.Unmarshal(frame.raw, &ack); err != nil {
				return closeWithError(fmt.Errorf("klient: decode subscribe_v2 ack: %w", err))
			}
			if ack.Code != 0 {
				return closeWithError(&SubscribeError{Code: ack.Code, Msg: ack.Msg})
			}
			if len(ack.Payload.NotFound) > 0 || !containsString(ack.Payload.Accepted, sessionID) {
				return closeWithError(&SubscribeError{
					Code: ack.Code, Msg: ack.Msg, Accepted: ack.Payload.Accepted, NotFound: ack.Payload.NotFound,
				})
			}
			ch := make(chan Event)
			loopCtx, loopCancel := context.WithCancel(ctx)
			if !c.activateTranscriptMapper(sessionID, loopCancel) {
				loopCancel()
				return closeWithError(fmt.Errorf("klient: session %s closed during subscribe", sessionID))
			}
			subscription := &Subscription{
				Events: ch, cancel: loopCancel, done: make(chan struct{}), cursor: mapper.cursorSnapshot(),
			}
			keepMapper = true
			go func() {
				defer close(subscription.done)
				c.readLoop(loopCtx, conn, ch, subscription, mapper, pending)
			}()
			return subscription, nil
		case "ping":
			if err := answerPing(handshakeCtx, conn, frame.Payload); err != nil {
				return closeWithError(err)
			}
		case "error":
			return closeWithError(handshakeProtocolError(frame.Payload))
		default:
			pending = append(pending, frame)
		}
	}
}

func (c *Client) wsURL() (string, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	switch {
	case strings.HasPrefix(base, "http://"):
		base = "ws" + strings.TrimPrefix(base, "http")
	case strings.HasPrefix(base, "https://"):
		base = "wss" + strings.TrimPrefix(base, "https")
	default:
		return "", fmt.Errorf("klient: unsupported BaseURL scheme %q", c.BaseURL)
	}
	return base + "/api/v1/ws", nil
}

func writeFrame(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("klient: marshal ws frame: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("klient: ws write: %w", err)
	}
	return nil
}

func readWSFrame(ctx context.Context, conn *websocket.Conn) (wsFrame, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return wsFrame{}, err
	}
	var frame wsFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return wsFrame{}, fmt.Errorf("klient: decode ws frame: %w", err)
	}
	if frame.Type == "" {
		return wsFrame{}, fmt.Errorf("klient: ws frame missing type")
	}
	frame.raw = data
	return frame, nil
}

func answerPing(ctx context.Context, conn *websocket.Conn, payload json.RawMessage) error {
	var p struct {
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("klient: decode ping: %w", err)
	}
	return writeFrame(ctx, conn, map[string]any{"type": "pong", "payload": map[string]string{"nonce": p.Nonce}})
}

func handshakeProtocolError(payload json.RawMessage) error {
	var p ProtocolErrorPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("klient: ws handshake error: %w", err)
	}
	return &SubscribeError{Code: p.Code, Msg: p.Msg}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

const rawFrameQueueSize = 64

var errRawFrameQueueFull = errors.New("klient: raw frame queue full")

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, ch chan<- Event, subscription *Subscription, mapper *transcriptEventMapper, pending []wsFrame) {
	defer close(ch)
	defer conn.Close(websocket.StatusNormalClosure, "")
	defer func() {
		c.putTranscriptMapper(mapper.sessionID, mapper)
	}()

	readerCtx, cancelReader := context.WithCancel(ctx)
	rawFrames := make(chan wsFrame, rawFrameQueueSize)
	readerResult := make(chan error, 1)
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		readerResult <- runWSReader(readerCtx, conn, pending, rawFrames)
		close(rawFrames)
	}()
	defer func() {
		cancelReader()
		conn.CloseNow()
		readerWG.Wait()
	}()

	process := func(frame wsFrame) bool {
		working := mapper.clone()
		events, fatal, err := mapWSFrame(working, frame)
		if err != nil {
			emitStreamError(ctx, ch, err)
			return false
		}
		for _, ev := range events {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return false
			}
		}
		mapper = working
		subscription.setCursor(mapper.cursorSnapshot())
		return !fatal
	}

	for frame := range rawFrames {
		if !process(frame) {
			return
		}
	}
	if err := <-readerResult; err != nil && ctx.Err() == nil {
		emitStreamError(ctx, ch, err)
	}
}

func runWSReader(ctx context.Context, conn *websocket.Conn, pending []wsFrame, rawFrames chan<- wsFrame) error {
	send := func(frame wsFrame) error {
		if frame.Type == "ping" {
			return answerPing(ctx, conn, frame.Payload)
		}
		select {
		case rawFrames <- frame:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn.CloseNow()
			return errRawFrameQueueFull
		}
	}
	for _, frame := range pending {
		if err := send(frame); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
	for {
		frame, err := readWSFrame(ctx, conn)
		if err != nil {
			if ctx.Err() != nil || websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return nil
			}
			return err
		}
		if err := send(frame); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func mapWSFrame(mapper *transcriptEventMapper, frame wsFrame) ([]Event, bool, error) {
	switch frame.Type {
	case "server_hello", "ack", "pong":
		return nil, false, nil
	case "error":
		var p ProtocolErrorPayload
		if err := json.Unmarshal(frame.Payload, &p); err != nil {
			return nil, true, fmt.Errorf("klient: decode protocol error: %w", err)
		}
		return []Event{{Type: TypeProtocolError, SessionID: frame.SessionID, AgentID: frame.AgentID, Payload: frame.Payload}}, p.Fatal, nil
	case "transcript.reset":
		if frame.SessionID != mapper.sessionID {
			return nil, false, fmt.Errorf("klient: transcript reset session=%s does not match %s", frame.SessionID, mapper.sessionID)
		}
		var p transcriptResetPayload
		if err := json.Unmarshal(frame.Payload, &p); err != nil {
			return nil, false, fmt.Errorf("klient: decode transcript reset: %w", err)
		}
		events, err := mapper.applyReset(p)
		indexTranscriptEvents(events)
		return events, false, err
	case "transcript.ops":
		if frame.SessionID != mapper.sessionID {
			return nil, false, fmt.Errorf("klient: transcript ops session=%s does not match %s", frame.SessionID, mapper.sessionID)
		}
		var p transcriptOpsPayload
		if err := json.Unmarshal(frame.Payload, &p); err != nil {
			return nil, false, fmt.Errorf("klient: decode transcript ops: %w", err)
		}
		events, err := mapper.applyOps(p)
		indexTranscriptEvents(events)
		return events, false, err
	case "agent":
		var p struct {
			Status         string `json:"status"`
			Model          string `json:"model"`
			Thinking       string `json:"thinking"`
			PermissionMode string `json:"permission_mode"`
			PlanMode       bool   `json:"plan_mode"`
			SwarmMode      bool   `json:"swarm_mode"`
			TowerMode      bool   `json:"tower_mode"`
		}
		if err := json.Unmarshal(frame.raw, &p); err != nil {
			return nil, false, fmt.Errorf("klient: decode agent frame: %w", err)
		}
		identity := EventIdentity{AgentID: frame.AgentID, SessionID: frame.SessionID}
		return []Event{newEvent(frame.SessionID, frame.AgentID, 0, TypeAgentStatus, AgentStatusPayload{
			EventIdentity:  identity,
			Status:         p.Status,
			Model:          p.Model,
			ThinkingEffort: p.Thinking,
			Permission:     p.PermissionMode,
			PlanMode:       p.PlanMode,
			SwarmMode:      p.SwarmMode,
			TowerMode:      p.TowerMode,
			Phase:          &AgentPhase{Kind: p.Status},
		})}, false, nil
	case "usage":
		var p struct {
			ContextTokens    int64 `json:"context_tokens"`
			MaxContextTokens int64 `json:"max_context_tokens"`
		}
		if err := json.Unmarshal(frame.raw, &p); err != nil {
			return nil, false, fmt.Errorf("klient: decode usage frame: %w", err)
		}
		return []Event{newEvent(frame.SessionID, frame.AgentID, 0, TypeUsage, UsageContextPayload{
			EventIdentity:    EventIdentity{AgentID: frame.AgentID, SessionID: frame.SessionID},
			ContextTokens:    p.ContextTokens,
			MaxContextTokens: p.MaxContextTokens,
		})}, false, nil
	case "task.list":
		var p struct {
			Tasks []Task `json:"tasks"`
		}
		if err := json.Unmarshal(frame.raw, &p); err != nil {
			return nil, false, fmt.Errorf("klient: decode task.list frame: %w", err)
		}
		return []Event{newEvent(frame.SessionID, frame.AgentID, 0, TypeTaskList, TaskListPayload{
			EventIdentity: EventIdentity{AgentID: frame.AgentID, SessionID: frame.SessionID},
			Tasks:         p.Tasks,
		})}, false, nil
	case "task":
		var task Task
		if err := json.Unmarshal(frame.raw, &task); err != nil {
			return nil, false, fmt.Errorf("klient: decode task frame: %w", err)
		}
		status := task.Status
		typ := TypeTaskStarted
		if task.Status == "ended" {
			typ = TypeTaskTerminated
			status = task.Outcome
			if status == "" {
				status = "lost"
			}
		}
		if task.Detached {
			if typ == TypeTaskStarted {
				typ = TypeBackgroundTaskStarted
			} else {
				typ = TypeBackgroundTaskTerminated
			}
		}
		return []Event{newEvent(frame.SessionID, frame.AgentID, 0, typ, TaskEventPayload{
			EventIdentity: EventIdentity{AgentID: frame.AgentID, SessionID: frame.SessionID},
			Info: TaskInfoPayload{
				TaskID:           task.ID,
				Status:           status,
				Detached:         task.Detached,
				RunInBackground:  task.Detached,
				Kind:             task.Kind,
				Description:      task.Description,
				ChildAgentID:     task.ChildAgentID,
				ParentToolCallID: task.ParentToolCallID,
				SwarmIndex:       task.SwarmIndex,
				Command:          task.Command,
				StartedAt:        task.StartedAt,
				EndedAt:          task.EndedAt,
				ResultSummary:    task.ResultSummary,
				Error:            task.Error,
				StateReason:      task.StateReason,
				Model:            task.Model,
				ThinkingEffort:   task.Thinking,
			},
		})}, false, nil
	case "task.output":
		var p struct {
			TaskID string `json:"task_id"`
			Offset int64  `json:"offset"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(frame.raw, &p); err != nil {
			return nil, false, fmt.Errorf("klient: decode task.output frame: %w", err)
		}
		return []Event{newEvent(frame.SessionID, frame.AgentID, 0, TypeTaskOutput, TaskOutputPayload{
			EventIdentity: EventIdentity{AgentID: frame.AgentID, SessionID: frame.SessionID},
			TaskID:        p.TaskID,
			Offset:        p.Offset,
			Text:          p.Text,
		})}, false, nil
	default:
		agentID := frame.AgentID
		sessionID := frame.SessionID
		if agentID == "" || sessionID == "" {
			var identity EventIdentity
			if json.Unmarshal(frame.Payload, &identity) == nil {
				if agentID == "" {
					agentID = identity.AgentID
				}
				if sessionID == "" {
					sessionID = identity.SessionID
				}
			}
		}
		return []Event{{Type: frame.Type, SessionID: sessionID, AgentID: agentID, Payload: frame.Payload}}, false, nil
	}
}

func indexTranscriptEvents(events []Event) {
	for i := range events {
		events[i].EventIndex = i
	}
}

func emitStreamError(ctx context.Context, ch chan<- Event, err error) {
	payload, _ := json.Marshal(map[string]string{"message": err.Error()})
	select {
	case ch <- Event{Type: TypeReadError, Payload: payload}:
	case <-ctx.Done():
	}
}
