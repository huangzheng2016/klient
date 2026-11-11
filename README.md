# klient

Lightweight Go client for the Kimi Agent Protocol served by Kimi Code 2.5.3 (`kimi web`), with acknowledged `subscribe_v2` subscriptions, per-agent transcript resumption, lifecycle APIs, and trace event normalization.

## Install

```bash
go get github.com/huangzheng2016/klient
```

## Features

- **Session & lifecycle REST API**:
  - `Healthz(ctx)` — probe `/api/v1/healthz`
  - `CreateSession(ctx, cwd)` — create a workspace session
  - `GetSession(ctx, sessionID)` — read session activity and metadata
  - `GetSnapshot(ctx, sessionID)` — read session state plus the main agent transcript and cursor
  - `GetAgentSnapshot(ctx, sessionID, agentID)` — read another agent transcript and cursor
  - `RestoreSession(ctx, sessionID)` / `ReloadSession(ctx, sessionID)` — run 2.5.3 lifecycle actions
  - `ListMCPServers(ctx)` — inspect configured MCP servers
  - `PostPrompt(ctx, sessionID, PromptRequest)` — submit a prompt
  - `SteerPrompts(ctx, sessionID, promptIDs)` — steer queued prompts into an active turn
  - `Abort(ctx, sessionID)` — abort an active session turn
  - `CloseSession(sessionID)` — cancel the active local subscription and release cached transcript state without deleting the server session
- **WebSocket event subscription (`/api/v1/ws`)**:
  - Performs `client_hello` and `subscribe_v2` handshakes and waits for both corresponding acknowledgements.
  - Rejects non-zero acknowledgements, missing sessions, unsupported protocol versions, and handshake timeouts.
  - Subscribes to all agents at `delta` grade and resumes with `transcript_since` from `Cursor.TranscriptSince`; a cursor returned by `Snapshot.Cursor()` also carries a private transcript baseline.
  - Maps `transcript.reset` and `transcript.ops` into assistant, thinking, tool, turn, prompt, error, and subagent events.
  - Tracks per-agent sequence cursors, suppresses replay duplicates, and detects sequence and append gaps.
  - Numbers each mapped transcript event with `EventIndex` from zero within its WS reset/ops frame. `(AgentID, Seq, EventIndex)` is the batch-scoped deduplication key for transcript events; lifecycle events with `Seq=0` do not use that key.
  - Applies external `transcript.reset` incrementally against cached state so old frames are not replayed, while explicit `op=reset` retains its clearing/undo semantics.
  - Commits cursor and cache only after every mapped event in a transcript batch has been received. Closing mid-batch replays that batch on reconnect, favoring duplicates over event loss.
  - Preserves split UTF-16 surrogate pairs across transcript messages without emitting unpaired surrogates.
  - Preserves transcript state across reconnects on the same `Client`; `Subscription.Cursor()` returns the latest committed per-agent cursor.
  - Rejects concurrent subscriptions to the same session on one `Client` with `ErrSessionSubscribed`.
  - Exposes `agent`, `usage`, `task.list`, `task`, and `task.output` as lifecycle events. `task.list` contains currently running tasks, not historical tasks.
  - Automatically answers server `ping` frames with `pong`.
- **Normalized trace mapper (`KAPMapper` / `TraceMapper`)**:
  - Maps events into standard trace kinds (`think`, `text`, `tool_call`, `tool_result`, `step_begin`, `turn_end`, `usage`, `error`, `end`).
  - Uses actual per-step token usage and separate context-window updates.
  - Filters prompt completion by main prompt ID and main agent ID.

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
	defer c.CloseSession(sess.ID)

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
			fmt.Printf("[%s] agent=%s seq=%d index=%d %s\n", kind, ev.AgentID, ev.Seq, ev.EventIndex, payload)
			if kind == "end" {
				break
			}
		}
	}
}
```

On reconnect, close the old subscription and pass `sub.Cursor()` back to `Subscribe` on the same `Client`. The cursor is batch-committed: after receiving one event from a multi-event batch, it may still refer to the preceding batch so that closing before the remaining events cannot skip them.

`EventIndex` is assigned after incremental mapping of each WS frame. Replaying an uncommitted frame starts from the same pre-frame state and therefore produces identical `(AgentID, Seq, EventIndex)` keys; it is not a cross-frame content identifier.

## License

MIT
