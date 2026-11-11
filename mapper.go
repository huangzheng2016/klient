package klient

import (
	"encoding/json"
)

type KAPMapper struct {
	MainPromptID  string
	models        map[string]string
	contextTokens map[string]int64
}

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
	case TypeTurnStepComplete:
		var p TurnStepCompletedPayload
		if err := ev.Decode(&p); err != nil || p.Usage == nil {
			return "", nil, false
		}
		cacheRead := p.Usage.InputCacheRead
		cacheCreation := p.Usage.InputCacheCreation
		in := p.Usage.InputOther + cacheRead + cacheCreation
		out := p.Usage.Output
		if in <= 0 && out <= 0 {
			return "", nil, false
		}
		agentID := kapMapperAgentID(ev.AgentID, p.AgentID)
		return "usage", kapPayload(map[string]any{
			"input_tokens": in, "output_tokens": out,
			"cache_read": cacheRead, "cache_creation": cacheCreation,
			"context_tokens": m.contextTokens[agentID], "model": m.models[agentID],
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
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		if m.models == nil {
			m.models = make(map[string]string)
		}
		m.models[kapMapperAgentID(ev.AgentID, p.AgentID)] = p.Model
		return "", nil, false
	case TypeUsage:
		var p UsageContextPayload
		if err := ev.Decode(&p); err != nil {
			return "", nil, false
		}
		if m.contextTokens == nil {
			m.contextTokens = make(map[string]int64)
		}
		m.contextTokens[kapMapperAgentID(ev.AgentID, p.AgentID)] = p.ContextTokens
		return "", nil, false
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
		if m.MainPromptID == "" || p.PromptID != m.MainPromptID ||
			(ev.AgentID != "" && ev.AgentID != "main") {
			return "", nil, false
		}
		return "end", kapPayload(map[string]any{
			"prompt_id": p.PromptID, "reason": p.Reason,
		}), true
	}
	return "", nil, false
}

func kapMapperAgentID(eventAgentID, payloadAgentID string) string {
	if eventAgentID != "" {
		return eventAgentID
	}
	if payloadAgentID != "" {
		return payloadAgentID
	}
	return "main"
}

func kapPayload(v map[string]any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
