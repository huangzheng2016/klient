package klient

import (
	"encoding/json"
)

type Cursor struct {
	Seq   int64  `json:"seq"`
	Epoch string `json:"epoch"`
}

type Event struct {
	Type      string
	Seq       int64
	Epoch     string
	SessionID string
	Payload   json.RawMessage
}

func (e Event) Decode(v any) error {
	return json.Unmarshal(e.Payload, v)
}

const TypeResyncRequired = "resync_required"

const TypeReadError = "klient.read_error"

const TypeProtocolError = "klient.protocol_error"

const (
	TypeTurnStarted              = "turn.started"
	TypeTurnEnded                = "turn.ended"
	TypeAssistantDelta           = "assistant.delta"
	TypeThinkingDelta            = "thinking.delta"
	TypeToolCallStarted          = "tool.call.started"
	TypeToolCallDelta            = "tool.call.delta"
	TypeToolProgress             = "tool.progress"
	TypeToolResult               = "tool.result"
	TypeTurnStepStarted          = "turn.step.started"
	TypeTurnStepComplete         = "turn.step.completed"
	TypeAgentStatus              = "agent.status.updated"
	TypePromptCompleted          = "prompt.completed"
	TypePromptAborted            = "prompt.aborted"
	TypeError                    = "error"
	TypeWarning                  = "warning"
	TypeWorkChanged              = "event.session.work_changed"
	TypeTaskStarted              = "task.started"
	TypeTaskTerminated           = "task.terminated"
	TypeTaskNotified             = "task.notified"
	TypeTaskWaitDelivered        = "task.waitDelivered"
	TypeBackgroundTaskStarted    = "background.task.started"
	TypeBackgroundTaskTerminated = "background.task.terminated"
)

type PromptOrigin struct {
	Kind string `json:"kind"`
}

type TurnStartedPayload struct {
	TurnID int64        `json:"turnId"`
	Origin PromptOrigin `json:"origin"`
	Prompt string       `json:"prompt,omitempty"`
}

type TurnEndedPayload struct {
	TurnID          int64         `json:"turnId"`
	Reason          string        `json:"reason"`
	Error           *ErrorPayload `json:"error,omitempty"`
	DurationMs      int64         `json:"durationMs,omitempty"`
	InterruptReason string        `json:"interruptReason,omitempty"`
	TraceID         string        `json:"traceId,omitempty"`
}

type AssistantDeltaPayload struct {
	TurnID int64  `json:"turnId"`
	Delta  string `json:"delta"`
}

type ThinkingDeltaPayload struct {
	TurnID int64  `json:"turnId"`
	Delta  string `json:"delta"`
}

type ToolCallStartedPayload struct {
	TurnID      int64           `json:"turnId"`
	ToolCallID  string          `json:"toolCallId"`
	Name        string          `json:"name"`
	Args        json.RawMessage `json:"args"`
	Description string          `json:"description,omitempty"`
}

type ToolCallDeltaPayload struct {
	TurnID        int64  `json:"turnId"`
	ToolCallID    string `json:"toolCallId"`
	Name          string `json:"name,omitempty"`
	ArgumentsPart string `json:"argumentsPart,omitempty"`
}

type ToolUpdate struct {
	Kind       string          `json:"kind"`
	Text       string          `json:"text,omitempty"`
	Percent    float64         `json:"percent,omitempty"`
	CustomKind string          `json:"customKind,omitempty"`
	CustomData json.RawMessage `json:"customData,omitempty"`
}

type ToolProgressPayload struct {
	TurnID     int64      `json:"turnId"`
	ToolCallID string     `json:"toolCallId"`
	Update     ToolUpdate `json:"update"`
}

type ToolResultPayload struct {
	TurnID     int64           `json:"turnId"`
	ToolCallID string          `json:"toolCallId"`
	Output     json.RawMessage `json:"output"`
	IsError    bool            `json:"isError,omitempty"`
	Synthetic  bool            `json:"synthetic,omitempty"`
}

type TurnStepPayload struct {
	TurnID int64  `json:"turnId"`
	Step   int64  `json:"step"`
	StepID string `json:"stepId,omitempty"`
}

type TurnStepCompletedPayload struct {
	TurnID       int64         `json:"turnId"`
	Step         int64         `json:"step"`
	StepID       string        `json:"stepId,omitempty"`
	Usage        *UsagePayload `json:"usage,omitempty"`
	FinishReason string        `json:"finishReason,omitempty"`
}

type UsagePayload struct {
	InputOther         int64 `json:"inputOther"`
	Output             int64 `json:"output"`
	InputCacheRead     int64 `json:"inputCacheRead"`
	InputCacheCreation int64 `json:"inputCacheCreation"`
}

type UsageStatusPayload struct {
	ByModel     map[string]UsagePayload `json:"byModel,omitempty"`
	CurrentTurn *UsagePayload           `json:"currentTurn,omitempty"`
	Total       *UsagePayload           `json:"total,omitempty"`
}

type AgentPhase struct {
	Kind   string `json:"kind"`
	TurnID int64  `json:"turnId,omitempty"`
	Step   int64  `json:"step,omitempty"`
	StepID string `json:"stepId,omitempty"`
}

type AgentStatusPayload struct {
	Model            string              `json:"model,omitempty"`
	ThinkingEffort   string              `json:"thinkingEffort,omitempty"`
	ContextTokens    int64               `json:"contextTokens,omitempty"`
	MaxContextTokens int64               `json:"maxContextTokens,omitempty"`
	ContextUsage     float64             `json:"contextUsage,omitempty"`
	PlanMode         bool                `json:"planMode,omitempty"`
	SwarmMode        bool                `json:"swarmMode,omitempty"`
	Permission       string              `json:"permission,omitempty"`
	Phase            *AgentPhase         `json:"phase,omitempty"`
	Usage            *UsageStatusPayload `json:"usage,omitempty"`
}

type PromptCompletedPayload struct {
	PromptID   string `json:"promptId"`
	FinishedAt string `json:"finishedAt"`
	Reason     string `json:"reason,omitempty"`
}

type PromptAbortedPayload struct {
	PromptID  string `json:"promptId"`
	AbortedAt string `json:"abortedAt"`
}

type ErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Name      string `json:"name,omitempty"`
	Retryable bool   `json:"retryable"`
}

type ProtocolErrorPayload struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Fatal bool   `json:"fatal"`
}

type WarningPayload struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type WorkChangedPayload struct {
	Busy           bool   `json:"busy"`
	MainTurnActive bool   `json:"main_turn_active"`
	LastTurnReason string `json:"last_turn_reason,omitempty"`
}
