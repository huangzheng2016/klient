package klient

import (
	"encoding/json"
	"strings"
	"testing"
)

func kapEvent(t *testing.T, typ string, payload string) Event {
	t.Helper()
	return Event{Type: typ, Seq: 7, Epoch: "e1", SessionID: "s1", Payload: json.RawMessage(payload)}
}

func TestKAPMapper_EventTable(t *testing.T) {
	cases := []struct {
		name         string
		typ          string
		payload      string
		mainPromptID string
		wantKind     string
		wantSkip     bool
		wantSubstrs  []string
	}{
		{
			name:     "thinking delta",
			typ:      TypeThinkingDelta,
			payload:  `{"turnId":1,"delta":"hmm"}`,
			wantKind: "think",
			wantSubstrs: []string{
				`"role":"reasoning"`, `"content":"hmm"`, `"turn_id":1`,
			},
		},
		{
			name:     "assistant delta",
			typ:      TypeAssistantDelta,
			payload:  `{"turnId":1,"delta":"你好"}`,
			wantKind: "text",
			wantSubstrs: []string{
				`"role":"assistant"`, `"content":"你好"`, `"turn_id":1`,
			},
		},
		{
			name:     "tool call started",
			typ:      TypeToolCallStarted,
			payload:  `{"turnId":1,"toolCallId":"c1","name":"query_events","args":{"limit":1},"description":"q"}`,
			wantKind: "tool_call",
			wantSubstrs: []string{
				`"role":"tool_call"`, `"tool_call_id":"c1"`, `"name":"query_events"`,
				`"arguments":{"limit":1}`, `"description":"q"`, `"turn_id":1`,
			},
		},
		{
			name:     "tool result",
			typ:      TypeToolResult,
			payload:  `{"turnId":1,"toolCallId":"c1","output":"rows","isError":false}`,
			wantKind: "tool_result",
			wantSubstrs: []string{
				`"role":"tool_result"`, `"tool_call_id":"c1"`, `"content":"rows"`, `"turn_id":1`,
			},
		},
		{
			name:     "step started",
			typ:      TypeTurnStepStarted,
			payload:  `{"turnId":1,"step":3,"stepId":"s-3"}`,
			wantKind: "step_begin",
			wantSubstrs: []string{
				`"turn_id":1`, `"step":3`, `"step_id":"s-3"`,
			},
		},
		{
			name:     "turn ended",
			typ:      TypeTurnEnded,
			payload:  `{"turnId":1,"reason":"completed","durationMs":42}`,
			wantKind: "turn_end",
			wantSubstrs: []string{
				`"turn_id":1`, `"reason":"completed"`, `"duration_ms":42`,
			},
		},
		{
			name:     "turn ended with error",
			typ:      TypeTurnEnded,
			payload:  `{"turnId":1,"reason":"failed","error":{"code":"provider.api_error","message":"boom","retryable":false}}`,
			wantKind: "turn_end",
			wantSubstrs: []string{
				`"reason":"failed"`, `"error":"boom"`,
			},
		},
		{
			name:     "usage",
			typ:      TypeAgentStatus,
			payload:  `{"model":"kimi-k2.6","contextTokens":99,"usage":{"total":{"inputOther":10,"output":5,"inputCacheRead":2,"inputCacheCreation":0}}}`,
			wantKind: "usage",
			wantSubstrs: []string{
				`"input_tokens":12`, `"output_tokens":5`, `"cache_read":2`, `"cache_creation":0`,
				`"context_tokens":99`, `"model":"kimi-k2.6"`,
			},
		},
		{
			name:     "error",
			typ:      TypeError,
			payload:  `{"code":"provider.api_error","message":"boom","retryable":false}`,
			wantKind: "error",
			wantSubstrs: []string{
				`"role":"error"`, `"message":"boom"`, `"code":"provider.api_error"`, `"retryable":false`,
			},
		},
		{
			name:     "protocol error",
			typ:      TypeProtocolError,
			payload:  `{"code":50001,"msg":"fatal boom","fatal":true}`,
			wantKind: "error",
			wantSubstrs: []string{
				`"role":"error"`, `"message":"fatal boom"`, `"code":50001`, `"fatal":true`,
			},
		},
		{
			name:         "prompt completed",
			typ:          TypePromptCompleted,
			payload:      `{"promptId":"p1","finishedAt":"2026-01-01T00:00:00Z","reason":"completed"}`,
			mainPromptID: "p1",
			wantKind:     "end",
			wantSubstrs: []string{
				`"prompt_id":"p1"`, `"reason":"completed"`,
			},
		},
		{
			name:         "sub-agent prompt completed ignored",
			typ:          TypePromptCompleted,
			payload:      `{"promptId":"sub-9","finishedAt":"2026-01-01T00:00:00Z","reason":"completed"}`,
			mainPromptID: "p1",
			wantSkip:     true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &KAPMapper{MainPromptID: c.mainPromptID}
			kind, payload, ok := m.Map(kapEvent(t, c.typ, c.payload))
			if c.wantSkip {
				if ok {
					t.Fatalf("event %s unexpectedly mapped to %q", c.typ, kind)
				}
				return
			}
			if !ok {
				t.Fatalf("not ok: %s %s", c.typ, c.payload)
			}
			if kind != c.wantKind {
				t.Fatalf("kind=%q want %q", kind, c.wantKind)
			}
			for _, want := range wantSubstrs(c.wantSubstrs) {
				if !strings.Contains(string(payload), want) {
					t.Fatalf("payload missing %q: %s", want, payload)
				}
			}
		})
	}
}

func wantSubstrs(s []string) []string { return s }

func TestKAPMapper_SkipsUnmappedAndMalformed(t *testing.T) {
	m := &KAPMapper{}
	for _, ev := range []Event{
		kapEvent(t, TypeTurnStarted, `{"turnId":1,"origin":{"kind":"user"}}`),
		kapEvent(t, TypeToolCallDelta, `{"turnId":1,"toolCallId":"c1","argumentsPart":"{}"}`),
		kapEvent(t, TypeToolProgress, `{"turnId":1,"toolCallId":"c1","update":{"kind":"stdout"}}`),
		kapEvent(t, TypeTurnStepComplete, `{"turnId":1,"step":1}`),
		kapEvent(t, TypePromptAborted, `{"promptId":"p1","abortedAt":"2026-01-01T00:00:00Z"}`),
		kapEvent(t, TypeWarning, `{"message":"w"}`),
		kapEvent(t, TypeWorkChanged, `{"busy":true,"main_turn_active":true}`),
		kapEvent(t, TypeResyncRequired, `{}`),
		kapEvent(t, "unknown.future", `{}`),
		kapEvent(t, TypeThinkingDelta, `{"turnId":1,"delta":""}`),
		kapEvent(t, TypeAssistantDelta, `{"turnId":1,"delta":""}`),
		kapEvent(t, TypeAssistantDelta, `not-json`),
		kapEvent(t, TypeAgentStatus, `{"model":"m"}`),
		kapEvent(t, TypeAgentStatus, `{"usage":{"currentTurn":{"output":1}}}`),
	} {
		if kind, _, ok := m.Map(ev); ok {
			t.Fatalf("event %s unexpectedly mapped to %q", ev.Type, kind)
		}
	}
}

func TestKAPMapper_UsageEmitsDeltas(t *testing.T) {
	m := &KAPMapper{}
	_, p1, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":10,"output":4}}}`))
	if !ok {
		t.Fatal("first usage not mapped")
	}
	if !strings.Contains(string(p1), `"input_tokens":10`) {
		t.Fatalf("first usage payload=%s", p1)
	}
	_, p2, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":12,"output":6}}}`))
	if !ok {
		t.Fatal("second usage not mapped")
	}
	if !strings.Contains(string(p2), `"input_tokens":2`) ||
		!strings.Contains(string(p2), `"output_tokens":2`) {
		t.Fatalf("second usage payload=%s, want delta 2/2", p2)
	}
	if _, _, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":12,"output":6}}}`)); ok {
		t.Fatal("unchanged usage snapshot should not emit")
	}
}

func TestKAPMapper_UsageEmitsCacheDeltas(t *testing.T) {
	m := &KAPMapper{}
	_, p1, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":10,"output":4,"inputCacheRead":10,"inputCacheCreation":4}}}`))
	if !ok {
		t.Fatal("first usage not mapped")
	}
	if !strings.Contains(string(p1), `"cache_read":10`) || !strings.Contains(string(p1), `"cache_creation":4`) {
		t.Fatalf("first usage payload=%s", p1)
	}
	_, p2, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":12,"output":6,"inputCacheRead":15,"inputCacheCreation":9}}}`))
	if !ok {
		t.Fatal("second usage not mapped")
	}
	if !strings.Contains(string(p2), `"cache_read":5`) || !strings.Contains(string(p2), `"cache_creation":5`) {
		t.Fatalf("second usage payload=%s, want cache deltas 5/5", p2)
	}
}

func TestKAPMapper_UsageCounterReset(t *testing.T) {
	m := &KAPMapper{}
	if _, _, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":100,"output":50,"inputCacheRead":40,"inputCacheCreation":10}}}`)); !ok {
		t.Fatal("first usage not mapped")
	}
	_, p2, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":10,"output":5,"inputCacheRead":4,"inputCacheCreation":1}}}`))
	if !ok {
		t.Fatal("usage after counter reset must be mapped, not dropped")
	}
	for _, want := range []string{`"input_tokens":15`, `"output_tokens":5`, `"cache_read":4`, `"cache_creation":1`} {
		if !strings.Contains(string(p2), want) {
			t.Fatalf("payload missing %q (negative deltas must not be emitted): %s", want, p2)
		}
	}
	_, p3, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":12,"output":7,"inputCacheRead":6,"inputCacheCreation":2}}}`))
	if !ok {
		t.Fatal("usage after reset baseline not mapped")
	}
	for _, want := range []string{`"input_tokens":5`, `"output_tokens":2`, `"cache_read":2`, `"cache_creation":1`} {
		if !strings.Contains(string(p3), want) {
			t.Fatalf("payload missing %q: %s", want, p3)
		}
	}
}

func TestKAPMapper_SkipFirstUsageOnResume(t *testing.T) {
	m := &KAPMapper{SkipFirstUsage: true}
	if _, _, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":1000,"output":500,"inputCacheRead":200,"inputCacheCreation":100}}}`)); ok {
		t.Fatal("first (historical cumulative) usage of resumed session must be skipped")
	}
	_, p2, ok := m.Map(kapEvent(t, TypeAgentStatus,
		`{"usage":{"total":{"inputOther":1010,"output":505,"inputCacheRead":210,"inputCacheCreation":105}}}`))
	if !ok {
		t.Fatal("second usage not mapped")
	}
	for _, want := range []string{`"input_tokens":25`, `"output_tokens":5`, `"cache_read":10`, `"cache_creation":5`} {
		if !strings.Contains(string(p2), want) {
			t.Fatalf("payload missing %q: %s", want, p2)
		}
	}
}
