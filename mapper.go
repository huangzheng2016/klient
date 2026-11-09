package klient

import (
	"encoding/json"
)

// KAPMapper normalizes raw KAP WebSocket events into standard trace event kinds
// ("think", "text", "tool_call", "tool_result", "step_begin", "turn_end",
// "usage", "error", "end"), converting cumulative KAP usage snapshots into
// per-update token deltas and filtering sub-agent prompt completions.
type KAPMapper struct {
	MainPromptID           string
	SkipFirstUsage         bool
	lastUsageIn            int64
	lastUsageOut           int64
	lastUsageCacheRead     int64
	lastUsageCacheCreation int64
	hasLastUsage           bool
}

// TraceMapper is an alias for KAPMapper.
type TraceMapper = KAPMapper

func (m *KAPMapper) Map(ev Event) (string, json.RawMessage, bool) {
	switch ev.Type {
	case TypeThinkingDelta:
		var p ThinkingDeltaPayload
		if err := ev.Decode(&p); err != nil || p.Delta == "" {
			return "", nil, false
		}
		return "think", kapPayload(map[string]any{
			"role": "reasoning", "content": p.Delta, "turn_id": p.TurnID,
		}), true
	case TypeAssistantDelta:
		var p AssistantDeltaPayload
		if err := ev.Decode(&p); err != nil || p.Delta == "" {
			return "", nil, false
		}
		return "text", kapPayload(map[string]any{
			"role": "assistant", "content": p.Delta, "turn_id": p.TurnID,
		}), true
	case TypeToolCallStarted:
		var p ToolCallStartedPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		args := json.RawMessage(p.Args)
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		return "tool_call", kapPayload(map[string]any{
			"role": "tool_call", "tool_call_id": p.ToolCallID, "name": p.Name,
			"arguments": args, "description": p.Description, "turn_id": p.TurnID,
		}), true
	case TypeToolResult:
		var p ToolResultPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		out := json.RawMessage(p.Output)
		if len(out) == 0 {
			out = json.RawMessage(`""`)
		}
		return "tool_result", kapPayload(map[string]any{
			"role": "tool_result", "tool_call_id": p.ToolCallID, "content": out,
			"is_error": p.IsError, "turn_id": p.TurnID,
		}), true
	case TypeTurnStepStarted:
		var p TurnStepPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		return "step_begin", kapPayload(map[string]any{
			"turn_id": p.TurnID, "step": p.Step, "step_id": p.StepID,
		}), true
	case TypeTurnEnded:
		var p TurnEndedPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		errMsg := ""
		if p.Error != nil {
			errMsg = p.Error.Message
		}
		return "turn_end", kapPayload(map[string]any{
			"turn_id": p.TurnID, "reason": p.Reason, "error": errMsg,
			"duration_ms": p.DurationMs, "interrupt_reason": p.InterruptReason,
		}), true
	case TypeAgentStatus:
		var p AgentStatusPayload
		if err := ev.Decode(&p); err != nil || p.Usage == nil || p.Usage.Total == nil {
			return "", nil, false
		}
		totalCacheRead := p.Usage.Total.InputCacheRead
		totalCacheCreation := p.Usage.Total.InputCacheCreation
		totalIn := p.Usage.Total.InputOther + totalCacheRead + totalCacheCreation
		totalOut := p.Usage.Total.Output
		in, out := totalIn, totalOut
		cacheRead, cacheCreation := totalCacheRead, totalCacheCreation
		if m.hasLastUsage && totalIn >= m.lastUsageIn && totalOut >= m.lastUsageOut &&
			totalCacheRead >= m.lastUsageCacheRead && totalCacheCreation >= m.lastUsageCacheCreation {
			in = totalIn - m.lastUsageIn
			out = totalOut - m.lastUsageOut
			cacheRead = totalCacheRead - m.lastUsageCacheRead
			cacheCreation = totalCacheCreation - m.lastUsageCacheCreation
		}
		first := !m.hasLastUsage
		m.lastUsageIn, m.lastUsageOut = totalIn, totalOut
		m.lastUsageCacheRead, m.lastUsageCacheCreation = totalCacheRead, totalCacheCreation
		m.hasLastUsage = true
		if first && m.SkipFirstUsage {
			return "", nil, false
		}
		if in <= 0 && out <= 0 {
			return "", nil, false
		}
		return "usage", kapPayload(map[string]any{
			"input_tokens": in, "output_tokens": out,
			"cache_read": cacheRead, "cache_creation": cacheCreation,
			"context_tokens": p.ContextTokens, "model": p.Model,
		}), true
	case TypeError:
		var p ErrorPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		return "error", kapPayload(map[string]any{
			"role": "error", "message": p.Message,
			"code": p.Code, "name": p.Name, "retryable": p.Retryable,
		}), true
	case TypeProtocolError:
		var p ProtocolErrorPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		return "error", kapPayload(map[string]any{
			"role": "error", "message": p.Msg,
			"code": p.Code, "fatal": p.Fatal,
		}), true
	case TypePromptCompleted:
		var p PromptCompletedPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		if m.MainPromptID == "" || p.PromptID != m.MainPromptID {
			return "", nil, false
		}
		return "end", kapPayload(map[string]any{
			"prompt_id": p.PromptID, "reason": p.Reason,
		}), true
	}
	return "", nil, false
}

func kapPayload(v map[string]any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
