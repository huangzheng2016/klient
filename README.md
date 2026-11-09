# klient

Lightweight Go client for the **Kimi Agent Protocol (KAP)** served by `kimi web` (REST API + WebSocket event stream), with cursor-based stream resumption, mid-turn prompt steering, and trace event normalization.

## Install

```bash
go get github.com/huangzheng2016/klient
```

## Features

- **Session & lifecycle REST API**:
  - `Healthz(ctx)` — probe `/api/v1/healthz`
  - `CreateSession(ctx, cwd)` — create a workspace session (`POST /api/v1/sessions`)
  - `GetSnapshot(ctx, sessionID)` — fetch session state, in-flight turn, and durable `{Seq, Epoch}` cursor (`GET /api/v1/sessions/:id/snapshot`)
  - `ListTasks(ctx, sessionID)` — list foreground/background tasks (`GET /api/v1/sessions/:id/tasks`)
  - `ListMCPServers(ctx, sessionID)` — inspect MCP servers and tool counts (`GET /api/v1/mcp/servers`)
  - `PostPrompt(ctx, sessionID, PromptRequest)` — submit a prompt (`POST /api/v1/sessions/:id/prompts`)
  - `SteerPrompts(ctx, sessionID, promptIDs)` — steer pending prompts into an active turn (`POST /api/v1/sessions/:id/prompts/:pid:steer`)
  - `Abort(ctx, sessionID)` — abort an active session turn (`POST /api/v1/sessions/:id:abort`)
- **WebSocket event subscription (`/api/v1/ws`)**:
  - Performs `client_hello` -> `server_hello` -> `subscribe` handshake with optional `{Seq, Epoch}` cursor resumption.
  - Automatically answers server `ping` frames with `pong`.
  - Surfaces `resync_required`, `klient.read_error`, and `klient.protocol_error` cleanly on the `Events` channel.
- **Normalized trace mapper (`KAPMapper` / `TraceMapper`)**:
  - Maps raw KAP events into standard trace event kinds (`think`, `text`, `tool_call`, `tool_result`, `step_begin`, `turn_end`, `usage`, `error`, `end`).
  - Converts cumulative KAP token usage snapshots into per-step deltas (including cache read/creation tokens and `SkipFirstUsage` when resuming an existing session).

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/huangzheng2016/klient"
)

func main() {
	ctx := context.Background()
	c := klient.New("http://127.0.0.1:54321", "")

	if err := c.Healthz(ctx); err != nil {
		log.Fatal(err)
	}

	sess, err := c.CreateSession(ctx, "/workspace")
	if err != nil {
		log.Fatal(err)
	}

	snap, err := c.GetSnapshot(ctx, sess.ID)
	if err != nil {
		log.Fatal(err)
	}

	sub, err := c.Subscribe(ctx, sess.ID, snap.Cursor())
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Close()

	receipt, err := c.PostPrompt(ctx, sess.ID, klient.PromptRequest{
		Text:           "Explain the repository layout.",
		PermissionMode: "yolo",
	})
	if err != nil {
		log.Fatal(err)
	}

	mapper := &klient.KAPMapper{MainPromptID: receipt.PromptID}
	for ev := range sub.Events {
		if kind, payload, ok := mapper.Map(ev); ok {
			fmt.Printf("[%s] seq=%d %s\n", kind, ev.Seq, payload)
			if kind == "end" {
				break
			}
		}
	}
}
```

## License

MIT
