package klient

import (
	"context"
	"encoding/json"
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
		SessionIDs []string            `json:"session_ids"`
		Cursors    map[string]wsCursor `json:"cursors,omitempty"`
	} `json:"payload"`
}

type wsCursor struct {
	Seq   int64  `json:"seq"`
	Epoch string `json:"epoch,omitempty"`
}

type wsFrame struct {
	Type      string          `json:"type"`
	Seq       *int64          `json:"seq"`
	Epoch     string          `json:"epoch"`
	SessionID string          `json:"session_id"`
	Payload   json.RawMessage `json:"payload"`
}

type Subscription struct {
	Events <-chan Event

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
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

func (c *Client) subscribe(ctx context.Context, sessionID string, cursor Cursor) (*Subscription, error) {
	wsURL, err := c.wsURL()
	if err != nil {
		return nil, err
	}

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

	var hello wsClientHello
	hello.Type = "client_hello"
	hello.ID = "1"
	hello.Payload.ClientID = "klient-go"
	if err := writeFrame(handshakeCtx, conn, hello); err != nil {
		conn.CloseNow()
		return nil, err
	}

	for {
		_, data, err := conn.Read(handshakeCtx)
		if err != nil {
			conn.CloseNow()
			return nil, fmt.Errorf("klient: ws wait server_hello: %w", err)
		}
		var frame wsFrame
		if json.Unmarshal(data, &frame) != nil {
			continue
		}
		if frame.Type == "server_hello" {
			break
		}
	}

	var sub wsSubscribe
	sub.Type = "subscribe"
	sub.ID = "2"
	sub.Payload.SessionIDs = []string{sessionID}
	if cursor.Seq != 0 || cursor.Epoch != "" {
		sub.Payload.Cursors = map[string]wsCursor{
			sessionID: {Seq: cursor.Seq, Epoch: cursor.Epoch},
		}
	}
	if err := writeFrame(handshakeCtx, conn, sub); err != nil {
		conn.CloseNow()
		return nil, err
	}

	ch := make(chan Event, 64)
	loopCtx, loopCancel := context.WithCancel(ctx)
	subscription := &Subscription{Events: ch, cancel: loopCancel, done: make(chan struct{})}
	go func() {
		defer close(subscription.done)
		readLoop(loopCtx, conn, ch)
	}()
	return subscription, nil
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

func readLoop(ctx context.Context, conn *websocket.Conn, ch chan<- Event) {
	defer close(ch)
	defer conn.Close(websocket.StatusNormalClosure, "")

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				payload, _ := json.Marshal(map[string]string{"message": err.Error()})
				select {
				case ch <- Event{Type: TypeReadError, Payload: payload}:
				case <-ctx.Done():
				}
			}
			return
		}
		var frame wsFrame
		if json.Unmarshal(data, &frame) != nil || frame.Type == "" {
			continue
		}

		var ev Event
		switch frame.Type {
		case "server_hello", "ack", "pong":
			continue
		case "ping":
			var p struct {
				Nonce string `json:"nonce"`
			}
			_ = json.Unmarshal(frame.Payload, &p)
			pong := map[string]any{"type": "pong", "payload": map[string]string{"nonce": p.Nonce}}
			if err := writeFrame(ctx, conn, pong); err != nil && ctx.Err() == nil {
				payload, _ := json.Marshal(map[string]string{"message": "pong write: " + err.Error()})
				select {
				case ch <- Event{Type: TypeReadError, Payload: payload}:
				case <-ctx.Done():
				}
				return
			}
			continue
		case TypeResyncRequired:
			var p struct {
				SessionID  string `json:"session_id"`
				CurrentSeq int64  `json:"current_seq"`
				Epoch      string `json:"epoch"`
			}
			_ = json.Unmarshal(frame.Payload, &p)
			ev = Event{
				Type:      TypeResyncRequired,
				Seq:       p.CurrentSeq,
				Epoch:     p.Epoch,
				SessionID: p.SessionID,
				Payload:   frame.Payload,
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
			return
		case TypeError:
			if frame.Seq == nil {
				ev = Event{Type: TypeProtocolError, Payload: frame.Payload}
				break
			}
			fallthrough
		default:
			var seq int64
			if frame.Seq != nil {
				seq = *frame.Seq
			}
			ev = Event{
				Type:      frame.Type,
				Seq:       seq,
				Epoch:     frame.Epoch,
				SessionID: frame.SessionID,
				Payload:   frame.Payload,
			}
		}

		select {
		case ch <- ev:
		case <-ctx.Done():
			return
		}
	}
}
