package klient

import (
	"encoding/json"
)

type Cursor struct {
	TranscriptSince map[string]int64 `json:"transcript_since,omitempty"`
	baseline        map[string]transcriptSnapshot
}

func (c Cursor) clone() Cursor {
	out := Cursor{
		TranscriptSince: make(map[string]int64, len(c.TranscriptSince)),
		baseline:        make(map[string]transcriptSnapshot, len(c.baseline)),
	}
	for agentID, seq := range c.TranscriptSince {
		out.TranscriptSince[agentID] = seq
	}
	for agentID, snapshot := range c.baseline {
		out.baseline[agentID] = snapshot
	}
	return out
}

type Event struct {
	Type       string
	Seq        int64
	EventIndex int
	SessionID  string
	AgentID    string
	Payload    json.RawMessage
}

func (e Event) Decode(v any) error {
	return json.Unmarshal(e.Payload, v)
}

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
	TypeTurnStepInterrupted      = "turn.step.interrupted"
	TypeTurnStepRetrying         = "turn.step.retrying"
	TypeAgentStatus              = "agent.status.updated"
	TypeUsage                    = "usage"
	TypePromptCompleted          = "prompt.completed"
	TypePromptAborted            = "prompt.aborted"
	TypeError                    = "error"
	TypeWarning                  = "warning"
	TypeWorkChanged              = "event.session.work_changed"
	TypeTaskStarted              = "task.started"
	TypeTaskTerminated           = "task.terminated"
	TypeTaskNotified             = "task.notified"
	TypeTaskList                 = "task.list"
	TypeTaskOutput               = "task.output"
	TypeBackgroundTaskStarted    = "background.task.started"
	TypeBackgroundTaskTerminated = "background.task.terminated"
	TypeSubagentSpawned          = "subagent.spawned"
)

type EventIdentity struct {
	AgentID   string `json:"agentId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

type PromptOrigin struct {
	Kind    string          `json:"kind"`
	TaskID  string          `json:"taskId,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type TurnStartedPayload struct {
	EventIdentity
	TurnID   string       `json:"turnId"`
	PromptID string       `json:"promptId,omitempty"`
	Origin   PromptOrigin `json:"origin"`
	Prompt   string       `json:"prompt,omitempty"`
}

type TurnEndedPayload struct {
	EventIdentity
	TurnID          string        `json:"turnId"`
	PromptID        string        `json:"promptId,omitempty"`
	Reason          string        `json:"reason"`
	Error           *ErrorPayload `json:"error,omitempty"`
	DurationMs      int64         `json:"durationMs,omitempty"`
	InterruptReason string        `json:"interruptReason,omitempty"`
	TraceID         string        `json:"traceId,omitempty"`
}

type AssistantDeltaPayload struct {
	EventIdentity
	TurnID string `json:"turnId"`
	Delta  string `json:"delta"`
}

type ThinkingDeltaPayload struct {
	EventIdentity
	TurnID string `json:"turnId"`
	Delta  string `json:"delta"`
}

type ToolCallStartedPayload struct {
	EventIdentity
	TurnID      string          `json:"turnId"`
	ToolCallID  string          `json:"toolCallId"`
	Name        string          `json:"name"`
	Args        json.RawMessage `json:"args"`
	Description string          `json:"description,omitempty"`
	Display     json.RawMessage `json:"display,omitempty"`
}

type ToolCallDeltaPayload struct {
	EventIdentity
	TurnID        string `json:"turnId"`
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
	EventIdentity
	TurnID     string     `json:"turnId"`
	ToolCallID string     `json:"toolCallId"`
	Update     ToolUpdate `json:"update"`
}

type ToolResultPayload struct {
	EventIdentity
	TurnID     string          `json:"turnId"`
	ToolCallID string          `json:"toolCallId"`
	Output     json.RawMessage `json:"output"`
	IsError    bool            `json:"isError,omitempty"`
	Synthetic  bool            `json:"synthetic,omitempty"`
}

type TurnStepPayload struct {
	EventIdentity
	TurnID string `json:"turnId"`
	Step   int64  `json:"step"`
	StepID string `json:"stepId,omitempty"`
}

type TurnStepCompletedPayload struct {
	EventIdentity
	TurnID       string        `json:"turnId"`
	Step         int64         `json:"step"`
	StepID       string        `json:"stepId,omitempty"`
	Usage        *UsagePayload `json:"usage,omitempty"`
	FinishReason string        `json:"finishReason,omitempty"`
	EndReason    string        `json:"endReason,omitempty"`
	EndMessage   string        `json:"endMessage,omitempty"`
}

type TurnStepRetryingPayload struct {
	EventIdentity
	TurnID        string `json:"turnId"`
	Step          int64  `json:"step"`
	StepID        string `json:"stepId,omitempty"`
	FailedAttempt int64  `json:"failedAttempt"`
	NextAttempt   int64  `json:"nextAttempt"`
	MaxAttempts   int64  `json:"maxAttempts"`
	DelayMs       int64  `json:"delayMs"`
	ErrorName     string `json:"errorName"`
	ErrorMessage  string `json:"errorMessage"`
	StatusCode    int64  `json:"statusCode,omitempty"`
}

type UsagePayload struct {
	InputOther         int64 `json:"inputOther"`
	Output             int64 `json:"output"`
	InputCacheRead     int64 `json:"inputCacheRead"`
	InputCacheCreation int64 `json:"inputCacheCreation"`
}

type AgentPhase struct {
	Kind   string `json:"kind"`
	TurnID string `json:"turnId,omitempty"`
	Step   int64  `json:"step,omitempty"`
	StepID string `json:"stepId,omitempty"`
}

type AgentStatusPayload struct {
	EventIdentity
	Status         string      `json:"status,omitempty"`
	Model          string      `json:"model,omitempty"`
	ThinkingEffort string      `json:"thinkingEffort,omitempty"`
	Permission     string      `json:"permission,omitempty"`
	PlanMode       bool        `json:"planMode,omitempty"`
	SwarmMode      bool        `json:"swarmMode,omitempty"`
	TowerMode      bool        `json:"towerMode,omitempty"`
	Phase          *AgentPhase `json:"phase,omitempty"`
}

type UsageContextPayload struct {
	EventIdentity
	ContextTokens    int64 `json:"contextTokens"`
	MaxContextTokens int64 `json:"maxContextTokens,omitempty"`
}

type PromptCompletedPayload struct {
	EventIdentity
	PromptID   string `json:"promptId"`
	FinishedAt string `json:"finishedAt"`
	Reason     string `json:"reason,omitempty"`
}

type PromptAbortedPayload struct {
	EventIdentity
	PromptID  string `json:"promptId"`
	AbortedAt string `json:"abortedAt"`
}

type ErrorPayload struct {
	EventIdentity
	Code      string `json:"code"`
	Message   string `json:"message"`
	Name      string `json:"name,omitempty"`
	Retryable bool   `json:"retryable"`
}

type ProtocolErrorPayload struct {
	Code      int             `json:"code"`
	Msg       string          `json:"msg"`
	Fatal     bool            `json:"fatal"`
	RequestID string          `json:"request_id,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}

type WarningPayload struct {
	EventIdentity
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type WorkChangedPayload struct {
	EventIdentity
	Busy               bool   `json:"busy"`
	MainTurnActive     bool   `json:"main_turn_active"`
	PendingInteraction string `json:"pending_interaction,omitempty"`
	LastTurnReason     string `json:"last_turn_reason,omitempty"`
}

type Task struct {
	ID               string `json:"task_id"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	Outcome          string `json:"outcome,omitempty"`
	Phase            string `json:"phase,omitempty"`
	Detached         bool   `json:"detached"`
	Description      string `json:"description,omitempty"`
	ChildAgentID     string `json:"child_agent_id,omitempty"`
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`
	SwarmIndex       *int64 `json:"swarm_index,omitempty"`
	Command          string `json:"command,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	EndedAt          string `json:"ended_at,omitempty"`
	ResultSummary    string `json:"result_summary,omitempty"`
	Error            string `json:"error,omitempty"`
	StateReason      string `json:"state_reason,omitempty"`
	Model            string `json:"model,omitempty"`
	Thinking         string `json:"thinking,omitempty"`
}

type TaskInfoPayload struct {
	TaskID           string `json:"taskId"`
	Status           string `json:"status"`
	Detached         bool   `json:"detached"`
	RunInBackground  bool   `json:"runInBackground"`
	Kind             string `json:"kind,omitempty"`
	Description      string `json:"description,omitempty"`
	ChildAgentID     string `json:"childAgentId,omitempty"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`
	SwarmIndex       *int64 `json:"swarmIndex,omitempty"`
	Command          string `json:"command,omitempty"`
	StartedAt        string `json:"startedAt,omitempty"`
	EndedAt          string `json:"endedAt,omitempty"`
	ResultSummary    string `json:"resultSummary,omitempty"`
	Error            string `json:"error,omitempty"`
	StateReason      string `json:"stateReason,omitempty"`
	Model            string `json:"model,omitempty"`
	ThinkingEffort   string `json:"thinkingEffort,omitempty"`
}

type TaskEventPayload struct {
	EventIdentity
	Info TaskInfoPayload `json:"info"`
}

type TaskListPayload struct {
	EventIdentity
	Tasks []Task `json:"tasks"`
}

type TaskOutputPayload struct {
	EventIdentity
	TaskID string `json:"taskId"`
	Offset int64  `json:"offset"`
	Text   string `json:"text"`
}

type SubagentSpawnedPayload struct {
	EventIdentity
	SubagentID       string `json:"subagentId"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`
	ParentAgentID    string `json:"parentAgentId,omitempty"`
	Role             string `json:"role,omitempty"`
}
