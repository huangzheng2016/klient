package klient

import (
	"encoding/json"
	"strings"
	"testing"
)

func kapEvent(t *testing.T, typ string, payload string) Event {
	t.Helper()
	return Event{Type: typ, Seq: 7, SessionID: "s1", AgentID: "main", Payload: json.RawMessage(payload)}
}

func TestKAPMapperEventTable(t *testing.T) {
	cases := []struct {
		name         string
		typ          string
		payload      string
		agentID      string
		mainPromptID string
		wantKind     string
		wantSkip     bool
		wantSubstrs  []string
	}{
		{
			name:     "thinking delta",
			typ:      TypeThinkingDelta,
			payload:  `{"turnId":"t1","delta":"hmm"}`,
			wantKind: "think",
			wantSubstrs: []string{
				`"role":"reasoning"`, `"content":"hmm"`, `"turn_id":"t1"`,
			},
		},
		{
			name:     "assistant delta",
			typ:      TypeAssistantDelta,
			payload:  `{"turnId":"t1","delta":"你好"}`,
			wantKind: "text",
			wantSubstrs: []string{
				`"role":"assistant"`, `"content":"你好"`, `"turn_id":"t1"`,
			},
		},
		{
			name:     "tool call started",
			typ:      TypeToolCallStarted,
			payload:  `{"turnId":"t1","toolCallId":"c1","name":"query_events","args":{"limit":1},"description":"q"}`,
			wantKind: "tool_call",
			wantSubstrs: []string{
				`"role":"tool_call"`, `"tool_call_id":"c1"`, `"name":"query_events"`,
				`"arguments":{"limit":1}`, `"description":"q"`, `"turn_id":"t1"`,
			},
		},
		{
			name:     "tool result",
			typ:      TypeToolResult,
			payload:  `{"turnId":"t1","toolCallId":"c1","output":"rows","isError":false}`,
			wantKind: "tool_result",
			wantSubstrs: []string{
				`"role":"tool_result"`, `"tool_call_id":"c1"`, `"content":"rows"`, `"turn_id":"t1"`,
			},
		},
		{
			name:     "step started",
			typ:      TypeTurnStepStarted,
			payload:  `{"turnId":"t1","step":3,"stepId":"t1.3"}`,
			wantKind: "step_begin",
			wantSubstrs: []string{
				`"turn_id":"t1"`, `"step":3`, `"step_id":"t1.3"`,
			},
		},
		{
			name:     "step usage",
			typ:      TypeTurnStepComplete,
			payload:  `{"turnId":"t1","step":1,"usage":{"inputOther":10,"output":5,"inputCacheRead":2,"inputCacheCreation":0}}`,
			wantKind: "usage",
			wantSubstrs: []string{
				`"input_tokens":12`, `"output_tokens":5`, `"cache_read":2`, `"cache_creation":0`,
			},
		},
		{
			name:     "turn ended",
			typ:      TypeTurnEnded,
			payload:  `{"turnId":"t1","reason":"completed","durationMs":42}`,
			wantKind: "turn_end",
			wantSubstrs: []string{
				`"turn_id":"t1"`, `"reason":"completed"`, `"duration_ms":42`,
			},
		},
		{
			name:     "turn ended with error",
			typ:      TypeTurnEnded,
			payload:  `{"turnId":"t1","reason":"failed","error":{"code":"provider.api_error","message":"boom","retryable":false}}`,
			wantKind: "turn_end",
			wantSubstrs: []string{
				`"reason":"failed"`, `"error":"boom"`,
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
			name:         "other prompt completed ignored",
			typ:          TypePromptCompleted,
			payload:      `{"promptId":"sub-9","finishedAt":"2026-01-01T00:00:00Z","reason":"completed"}`,
			mainPromptID: "p1",
			wantSkip:     true,
		},
		{
			name:         "subagent completion with reused prompt id ignored",
			typ:          TypePromptCompleted,
			payload:      `{"promptId":"p1","finishedAt":"2026-01-01T00:00:00Z","reason":"completed"}`,
			agentID:      "agent-0",
			mainPromptID: "p1",
			wantSkip:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &KAPMapper{MainPromptID: tc.mainPromptID}
			ev := kapEvent(t, tc.typ, tc.payload)
			if tc.agentID != "" {
				ev.AgentID = tc.agentID
			}
			kind, payload, ok := m.Map(ev)
			if tc.wantSkip {
				if ok {
					t.Fatalf("event %s unexpectedly mapped to %q", tc.typ, kind)
				}
				return
			}
			if !ok {
				t.Fatalf("not ok: %s %s", tc.typ, tc.payload)
			}
			if kind != tc.wantKind {
				t.Fatalf("kind=%q want %q", kind, tc.wantKind)
			}
			for _, want := range tc.wantSubstrs {
				if !strings.Contains(string(payload), want) {
					t.Fatalf("payload missing %q: %s", want, payload)
				}
			}
		})
	}
}

func TestKAPMapperSkipsUnmappedAndMalformed(t *testing.T) {
	m := &KAPMapper{}
	for _, ev := range []Event{
		kapEvent(t, TypeTurnStarted, `{"turnId":"t1","origin":{"kind":"user"}}`),
		kapEvent(t, TypeToolCallDelta, `{"turnId":"t1","toolCallId":"c1","argumentsPart":"{}"}`),
		kapEvent(t, TypeToolProgress, `{"turnId":"t1","toolCallId":"c1","update":{"kind":"stdout"}}`),
		kapEvent(t, TypeTurnStepInterrupted, `{"turnId":"t1","step":1,"state":"interrupted"}`),
		kapEvent(t, TypePromptAborted, `{"promptId":"p1","abortedAt":"2026-01-01T00:00:00Z"}`),
		kapEvent(t, TypeWarning, `{"message":"w"}`),
		kapEvent(t, TypeWorkChanged, `{"busy":true,"main_turn_active":true}`),
		kapEvent(t, TypeTaskList, `{"tasks":[]}`),
		kapEvent(t, "unknown.future", `{}`),
		kapEvent(t, TypeThinkingDelta, `{"turnId":"t1","delta":""}`),
		kapEvent(t, TypeAssistantDelta, `{"turnId":"t1","delta":""}`),
		kapEvent(t, TypeAssistantDelta, `not-json`),
		kapEvent(t, TypeTurnStepComplete, `{"turnId":"t1","step":1}`),
		kapEvent(t, TypeTurnStepComplete, `{"turnId":"t1","step":1,"usage":{"inputOther":0,"output":0,"inputCacheCreation":0}}`),
	} {
		if kind, _, ok := m.Map(ev); ok {
			t.Fatalf("event %s unexpectedly mapped to %q", ev.Type, kind)
		}
	}
}

func TestKAPMapperStepUsageIsNotCumulative(t *testing.T) {
	m := &KAPMapper{}
	_, first, ok := m.Map(kapEvent(t, TypeTurnStepComplete,
		`{"turnId":"t1","step":1,"usage":{"inputOther":10,"output":4,"inputCacheRead":10,"inputCacheCreation":4}}`))
	if !ok {
		t.Fatal("first step usage not mapped")
	}
	if !strings.Contains(string(first), `"input_tokens":24`) || !strings.Contains(string(first), `"cache_read":10`) {
		t.Fatalf("first usage payload=%s", first)
	}
	_, second, ok := m.Map(kapEvent(t, TypeTurnStepComplete,
		`{"turnId":"t1","step":2,"usage":{"inputOther":12,"output":6,"inputCacheRead":15,"inputCacheCreation":9}}`))
	if !ok {
		t.Fatal("second step usage not mapped")
	}
	for _, want := range []string{`"input_tokens":36`, `"output_tokens":6`, `"cache_read":15`, `"cache_creation":9`} {
		if !strings.Contains(string(second), want) {
			t.Fatalf("second usage payload missing %q: %s", want, second)
		}
	}
}

func TestKAPMapperUsageIncludesAgentContext(t *testing.T) {
	m := &KAPMapper{}
	m.Map(kapEvent(t, TypeAgentStatus, `{"status":"running","model":"kimi-k2.6"}`))
	m.Map(kapEvent(t, TypeUsage, `{"contextTokens":99,"maxContextTokens":200000}`))
	_, payload, ok := m.Map(kapEvent(t, TypeTurnStepComplete,
		`{"turnId":"t1","step":1,"usage":{"inputOther":10,"output":5,"inputCacheRead":2,"inputCacheCreation":0}}`))
	if !ok {
		t.Fatal("step usage not mapped")
	}
	for _, want := range []string{`"context_tokens":99`, `"model":"kimi-k2.6"`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("usage payload missing %q: %s", want, payload)
		}
	}
}

func TestKAPMapperUsageIsolatedByAgent(t *testing.T) {
	m := &KAPMapper{}
	mainStatus := kapEvent(t, TypeAgentStatus, `{"status":"running","model":"main-model"}`)
	mainStatus.AgentID = "main"
	m.Map(mainStatus)
	childStatus := kapEvent(t, TypeAgentStatus, `{"status":"running","model":"child-model"}`)
	childStatus.AgentID = "agent-0"
	m.Map(childStatus)
	mainUsage := kapEvent(t, TypeUsage, `{"contextTokens":100}`)
	mainUsage.AgentID = "main"
	m.Map(mainUsage)
	childUsage := kapEvent(t, TypeUsage, `{"contextTokens":200}`)
	childUsage.AgentID = "agent-0"
	m.Map(childUsage)
	for _, tc := range []struct {
		agentID string
		model   string
		context string
	}{
		{agentID: "agent-0", model: `"model":"child-model"`, context: `"context_tokens":200`},
		{agentID: "main", model: `"model":"main-model"`, context: `"context_tokens":100`},
	} {
		ev := kapEvent(t, TypeTurnStepComplete,
			`{"turnId":"t1","step":1,"usage":{"inputOther":1,"output":1,"inputCacheCreation":0}}`)
		ev.AgentID = tc.agentID
		_, payload, ok := m.Map(ev)
		if !ok {
			t.Fatalf("usage for %s not mapped", tc.agentID)
		}
		if !strings.Contains(string(payload), tc.model) || !strings.Contains(string(payload), tc.context) {
			t.Fatalf("usage for %s = %s", tc.agentID, payload)
		}
	}
}
