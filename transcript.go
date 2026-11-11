package klient

import (
	"encoding/json"
	"fmt"
	"unicode/utf16"
)

type TranscriptTurn struct {
	Kind            string           `json:"kind"`
	TurnID          string           `json:"turnId"`
	TriggerPromptID string           `json:"triggerPromptId,omitempty"`
	Ordinal         int64            `json:"ordinal"`
	State           string           `json:"state"`
	Origin          PromptOrigin     `json:"origin"`
	Prompt          string           `json:"prompt,omitempty"`
	AttachmentIDs   []string         `json:"attachmentIds,omitempty"`
	Steps           []TranscriptStep `json:"steps,omitempty"`
	StartedAt       string           `json:"startedAt,omitempty"`
	EndedAt         string           `json:"endedAt,omitempty"`
	Usage           json.RawMessage  `json:"usage,omitempty"`
	DurationMs      int64            `json:"durationMs,omitempty"`
	Error           string           `json:"error,omitempty"`
	Wedged          bool             `json:"wedged,omitempty"`
}

type TranscriptStepRetry struct {
	FailedAttempt int64  `json:"failedAttempt"`
	NextAttempt   int64  `json:"nextAttempt"`
	MaxAttempts   int64  `json:"maxAttempts"`
	DelayMs       int64  `json:"delayMs"`
	ErrorName     string `json:"errorName"`
	ErrorMessage  string `json:"errorMessage"`
	StatusCode    int64  `json:"statusCode,omitempty"`
}

type TranscriptStep struct {
	Kind         string               `json:"kind"`
	StepID       string               `json:"stepId"`
	TurnID       string               `json:"turnId"`
	Ordinal      int64                `json:"ordinal"`
	State        string               `json:"state"`
	Frames       []TranscriptFrame    `json:"frames,omitempty"`
	StartedAt    string               `json:"startedAt,omitempty"`
	EndedAt      string               `json:"endedAt,omitempty"`
	Usage        *UsagePayload        `json:"usage,omitempty"`
	FinishReason string               `json:"finishReason,omitempty"`
	LLMTiming    json.RawMessage      `json:"llmTiming,omitempty"`
	Retry        *TranscriptStepRetry `json:"retry,omitempty"`
	EndReason    string               `json:"endReason,omitempty"`
	EndMessage   string               `json:"endMessage,omitempty"`
}

type TranscriptAgentRef struct {
	AgentID string `json:"agentId"`
	Role    string `json:"role,omitempty"`
}

type TranscriptFrame struct {
	Kind           string `json:"kind"`
	FrameID        string `json:"frameId"`
	textUnits      []uint16
	inputTextUnits []uint16
	Role           string               `json:"role,omitempty"`
	Text           string               `json:"text,omitempty"`
	TaskID         string               `json:"taskId,omitempty"`
	PromptIDs      []string             `json:"promptIds,omitempty"`
	AttachmentIDs  []string             `json:"attachmentIds,omitempty"`
	ToolCallID     string               `json:"toolCallId,omitempty"`
	Name           string               `json:"name,omitempty"`
	View           string               `json:"view,omitempty"`
	State          string               `json:"state,omitempty"`
	Input          json.RawMessage      `json:"input,omitempty"`
	Output         json.RawMessage      `json:"output,omitempty"`
	Display        json.RawMessage      `json:"display,omitempty"`
	Error          string               `json:"error,omitempty"`
	InputText      string               `json:"inputText,omitempty"`
	Progress       *ToolUpdate          `json:"progress,omitempty"`
	ApprovalID     string               `json:"approvalId,omitempty"`
	AgentRefs      []TranscriptAgentRef `json:"agentRefs,omitempty"`
	Level          string               `json:"level,omitempty"`
	Source         string               `json:"source,omitempty"`
	Message        string               `json:"message,omitempty"`
	Detail         json.RawMessage      `json:"detail,omitempty"`
}

type TranscriptPrompt struct {
	PromptID       string           `json:"promptId"`
	Status         string           `json:"status"`
	UserMessageID  string           `json:"userMessageId,omitempty"`
	Content        json.RawMessage  `json:"content,omitempty"`
	ClientMetadata []map[string]any `json:"clientMetadata,omitempty"`
	CreatedAt      string           `json:"createdAt"`
	FinishedAt     string           `json:"finishedAt,omitempty"`
	SteeredAt      string           `json:"steeredAt,omitempty"`
}

type TranscriptItem struct {
	Kind            string           `json:"kind"`
	TurnID          string           `json:"turnId,omitempty"`
	TriggerPromptID string           `json:"triggerPromptId,omitempty"`
	Ordinal         int64            `json:"ordinal,omitempty"`
	State           string           `json:"state,omitempty"`
	Origin          PromptOrigin     `json:"origin,omitempty"`
	Prompt          string           `json:"prompt,omitempty"`
	AttachmentIDs   []string         `json:"attachmentIds,omitempty"`
	Steps           []TranscriptStep `json:"steps,omitempty"`
	StartedAt       string           `json:"startedAt,omitempty"`
	EndedAt         string           `json:"endedAt,omitempty"`
	Usage           json.RawMessage  `json:"usage,omitempty"`
	DurationMs      int64            `json:"durationMs,omitempty"`
	Error           string           `json:"error,omitempty"`
	Wedged          bool             `json:"wedged,omitempty"`
	MarkerID        string           `json:"markerId,omitempty"`
	Marker          string           `json:"marker,omitempty"`
	Payload         json.RawMessage  `json:"payload,omitempty"`
	At              string           `json:"at,omitempty"`
}

type AgentDescriptor struct {
	AgentID       string `json:"agentId"`
	Type          string `json:"type,omitempty"`
	ParentAgentID string `json:"parentAgentId,omitempty"`
	Label         string `json:"label,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	DisposedAt    string `json:"disposedAt,omitempty"`
}

type transcriptSnapshot struct {
	Items        []TranscriptItem   `json:"items"`
	Interactions []json.RawMessage  `json:"interactions"`
	Attachments  []json.RawMessage  `json:"attachments"`
	Prompts      []TranscriptPrompt `json:"prompts"`
	HasMoreOlder bool               `json:"hasMoreOlder,omitempty"`
}

type transcriptAppendTarget struct {
	Type    string `json:"type"`
	TurnID  string `json:"turnId"`
	StepID  string `json:"stepId"`
	FrameID string `json:"frameId"`
}

type transcriptOperation struct {
	Op          string `json:"op"`
	textUnits   []uint16
	AgentID     string                 `json:"agentId,omitempty"`
	Snapshot    *transcriptSnapshot    `json:"snapshot,omitempty"`
	Turn        TranscriptTurn         `json:"turn,omitempty"`
	TurnID      string                 `json:"turnId,omitempty"`
	Step        TranscriptStep         `json:"step,omitempty"`
	StepID      string                 `json:"stepId,omitempty"`
	Frame       TranscriptFrame        `json:"frame,omitempty"`
	Target      transcriptAppendTarget `json:"target,omitempty"`
	Offset      int64                  `json:"offset,omitempty"`
	Text        string                 `json:"text,omitempty"`
	Item        TranscriptItem         `json:"item,omitempty"`
	BeforeTurn  *int64                 `json:"beforeTurn,omitempty"`
	Interaction json.RawMessage        `json:"interaction,omitempty"`
	Attachment  json.RawMessage        `json:"attachment,omitempty"`
	Prompt      TranscriptPrompt       `json:"prompt,omitempty"`
	IDs         []string               `json:"ids,omitempty"`
}

type transcriptResetPayload struct {
	AgentID      string             `json:"agent_id"`
	Snapshot     transcriptSnapshot `json:"snapshot"`
	HasMoreOlder bool               `json:"has_more_older"`
	Seq          *int64             `json:"seq"`
}

type transcriptOpsPayload struct {
	AgentID string                `json:"agent_id"`
	Ops     []transcriptOperation `json:"ops"`
	Seq     *int64                `json:"seq"`
}

type TranscriptGapError struct {
	AgentID  string
	Expected int64
	Got      int64
	Detail   string
}

func (e *TranscriptGapError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("klient: transcript gap agent=%s expected=%d got=%d: %s", e.AgentID, e.Expected, e.Got, e.Detail)
	}
	return fmt.Sprintf("klient: transcript seq gap agent=%s expected=%d got=%d", e.AgentID, e.Expected, e.Got)
}

type turnEventState struct {
	turn    TranscriptTurn
	started bool
	ended   bool
}

type stepEventState struct {
	step        TranscriptStep
	started     bool
	terminalKey string
	retryKey    string
}

type frameEventState struct {
	turnID         string
	stepID         string
	frame          TranscriptFrame
	textUnits      []uint16
	pendingText    []uint16
	inputTextUnits []uint16
	pendingInput   []uint16
	started        bool
	resultEmitted  bool
	progressKey    string
}

type agentEventState struct {
	initialized bool
	seq         int64
	turns       map[string]*turnEventState
	steps       map[string]*stepEventState
	frames      map[string]*frameEventState
	prompts     map[string]TranscriptPrompt
}

func newAgentEventState() *agentEventState {
	return &agentEventState{
		turns:   make(map[string]*turnEventState),
		steps:   make(map[string]*stepEventState),
		frames:  make(map[string]*frameEventState),
		prompts: make(map[string]TranscriptPrompt),
	}
}

type transcriptEventMapper struct {
	sessionID   string
	cursor      Cursor
	agents      map[string]*agentEventState
	promptEmits map[string]string
	resetKeys   map[string]string
}

func newTranscriptEventMapper(sessionID string, cursor Cursor) *transcriptEventMapper {
	if cursor.TranscriptSince == nil {
		cursor.TranscriptSince = make(map[string]int64)
	}
	m := &transcriptEventMapper{
		sessionID:   sessionID,
		cursor:      cursor.clone(),
		agents:      make(map[string]*agentEventState),
		promptEmits: make(map[string]string),
		resetKeys:   make(map[string]string),
	}
	for agentID, snapshot := range cursor.baseline {
		seq, ok := cursor.TranscriptSince[agentID]
		if !ok {
			seq = cursor.TranscriptSince["*"]
		}
		state := newAgentEventState()
		state.initialized = true
		state.seq = seq
		m.agents[agentID] = state
		m.seedAgentSnapshot(agentID, state, snapshot)
		m.resetKeys[agentID] = transcriptResetKey(seq, snapshot)
	}
	return m
}

func (m *transcriptEventMapper) matches(cursor Cursor) bool {
	if len(cursor.TranscriptSince) == 0 {
		return true
	}
	for agentID, state := range m.agents {
		seq, ok := cursor.TranscriptSince[agentID]
		if !ok {
			seq, ok = cursor.TranscriptSince["*"]
		}
		if ok && state.seq < seq {
			return false
		}
	}
	return true
}

func (m *transcriptEventMapper) cursorSnapshot() Cursor {
	return m.cursor.clone()
}

func (m *transcriptEventMapper) clone() *transcriptEventMapper {
	out := &transcriptEventMapper{
		sessionID:   m.sessionID,
		cursor:      m.cursor.clone(),
		agents:      make(map[string]*agentEventState, len(m.agents)),
		promptEmits: make(map[string]string, len(m.promptEmits)),
		resetKeys:   make(map[string]string, len(m.resetKeys)),
	}
	for agentID, state := range m.agents {
		cloned := newAgentEventState()
		cloned.initialized = state.initialized
		cloned.seq = state.seq
		for id, current := range state.turns {
			copy := *current
			cloned.turns[id] = &copy
		}
		for id, current := range state.steps {
			copy := *current
			cloned.steps[id] = &copy
		}
		for id, current := range state.frames {
			copy := *current
			copy.textUnits = cloneUTF16(current.textUnits)
			copy.pendingText = cloneUTF16(current.pendingText)
			copy.inputTextUnits = cloneUTF16(current.inputTextUnits)
			copy.pendingInput = cloneUTF16(current.pendingInput)
			cloned.frames[id] = &copy
		}
		for id, prompt := range state.prompts {
			cloned.prompts[id] = prompt
		}
		out.agents[agentID] = cloned
	}
	for id, value := range m.promptEmits {
		out.promptEmits[id] = value
	}
	for id, value := range m.resetKeys {
		out.resetKeys[id] = value
	}
	return out
}

func (m *transcriptEventMapper) seedAgentSnapshot(agentID string, state *agentEventState, snapshot transcriptSnapshot) {
	for _, item := range snapshot.Items {
		if item.Kind != "turn" {
			continue
		}
		turn := transcriptTurnFromItem(item)
		terminal := terminalTurnState(turn.State)
		state.turns[turn.TurnID] = &turnEventState{
			turn: turn, started: turn.State == "running" || terminal, ended: terminal,
		}
		for _, step := range turn.Steps {
			terminalStep := step.State == "completed" || step.State == "interrupted" || step.State == "failed"
			stepState := &stepEventState{
				step: step, started: step.State == "running" || terminalStep,
			}
			if terminalStep {
				stepState.terminalKey = transcriptStepTerminalKey(step)
			}
			if step.Retry != nil {
				stepState.retryKey = transcriptStepRetryKey(step.Retry)
			}
			state.steps[stepKey(turn.TurnID, step.StepID)] = stepState
			for _, frame := range step.Frames {
				frameState := &frameEventState{
					turnID: turn.TurnID, stepID: step.StepID, frame: frame,
					started: len(frame.Input) > 0, resultEmitted: frame.State == "done" || frame.State == "error",
				}
				frameState.textUnits = transcriptFrameTextUnits(frame)
				frameState.pendingText = trailingHighSurrogate(frameState.textUnits)
				frameState.inputTextUnits = transcriptFrameInputUnits(frame)
				frameState.pendingInput = trailingHighSurrogate(frameState.inputTextUnits)
				if frame.Progress != nil {
					progressKey, _ := json.Marshal(frame.Progress)
					frameState.progressKey = string(progressKey)
				}
				state.frames[frameKey(turn.TurnID, step.StepID, frame.FrameID)] = frameState
			}
		}
	}
	for _, prompt := range snapshot.Prompts {
		state.prompts[prompt.PromptID] = prompt
		if terminalPromptState(prompt.Status) {
			m.promptEmits[agentID+"\x00"+prompt.PromptID] = prompt.Status + "\x00" + prompt.FinishedAt
		}
	}
}

func (m *transcriptEventMapper) applyReset(p transcriptResetPayload) ([]Event, error) {
	if p.AgentID == "" {
		return nil, fmt.Errorf("klient: transcript reset missing agent_id")
	}
	seq := int64(0)
	if p.Seq != nil {
		seq = *p.Seq
	}
	snapshot := p.Snapshot
	snapshot.HasMoreOlder = snapshot.HasMoreOlder || p.HasMoreOlder
	resetKey := transcriptResetKey(seq, snapshot)
	if m.resetKeys[p.AgentID] == resetKey {
		return nil, nil
	}
	m.resetKeys[p.AgentID] = resetKey
	state := m.agents[p.AgentID]
	if state == nil {
		state = newAgentEventState()
		m.agents[p.AgentID] = state
	}
	if seq < state.seq {
		return nil, nil
	}
	state.initialized = true
	state.seq = seq
	m.cursor.TranscriptSince[p.AgentID] = state.seq
	return m.applySnapshot(p.AgentID, state, snapshot, state.seq)
}

func (m *transcriptEventMapper) applyResetOperation(agentID string, snapshot transcriptSnapshot, seq int64) ([]Event, error) {
	state := newAgentEventState()
	state.initialized = true
	state.seq = seq
	m.agents[agentID] = state
	return m.applySnapshot(agentID, state, snapshot, seq)
}

func (m *transcriptEventMapper) applyOps(p transcriptOpsPayload) ([]Event, error) {
	if p.AgentID == "" {
		return nil, fmt.Errorf("klient: transcript ops missing agent_id")
	}
	if p.Seq == nil {
		return nil, fmt.Errorf("klient: transcript ops agent=%s missing seq", p.AgentID)
	}
	state := m.agents[p.AgentID]
	if state == nil {
		seq, ok := m.cursor.TranscriptSince[p.AgentID]
		if !ok {
			seq, ok = m.cursor.TranscriptSince["*"]
		}
		if !ok {
			return nil, fmt.Errorf("klient: transcript ops before reset agent=%s seq=%d", p.AgentID, *p.Seq)
		}
		state = newAgentEventState()
		state.initialized = true
		state.seq = seq
		m.agents[p.AgentID] = state
	}
	if *p.Seq <= state.seq {
		return nil, nil
	}
	if state.initialized && *p.Seq != state.seq+1 {
		return nil, &TranscriptGapError{AgentID: p.AgentID, Expected: state.seq + 1, Got: *p.Seq}
	}
	events := make([]Event, 0)
	for _, op := range p.Ops {
		state = m.agents[p.AgentID]
		mapped, err := m.applyOperation(p.AgentID, state, op, *p.Seq)
		if err != nil {
			return nil, err
		}
		events = append(events, mapped...)
	}
	state = m.agents[p.AgentID]
	state.seq = *p.Seq
	m.cursor.TranscriptSince[p.AgentID] = *p.Seq
	return events, nil
}

func (m *transcriptEventMapper) applySnapshot(agentID string, state *agentEventState, snapshot transcriptSnapshot, seq int64) ([]Event, error) {
	events := make([]Event, 0)
	for _, item := range snapshot.Items {
		if item.Kind == "marker" {
			mapped, err := m.applyMarker(agentID, item, seq)
			if err != nil {
				return nil, err
			}
			events = append(events, mapped...)
			continue
		}
		if item.Kind != "turn" {
			continue
		}
		turn := transcriptTurnFromItem(item)
		terminal := terminalTurnState(turn.State)
		if terminal {
			start := turn
			start.State = "running"
			start.EndedAt = ""
			start.DurationMs = 0
			start.Error = ""
			mapped, err := m.applyTurn(agentID, state, start, seq)
			if err != nil {
				return nil, err
			}
			events = append(events, mapped...)
		} else {
			mapped, err := m.applyTurn(agentID, state, turn, seq)
			if err != nil {
				return nil, err
			}
			events = append(events, mapped...)
		}
		for _, step := range turn.Steps {
			terminalStep := step.State == "completed" || step.State == "interrupted" || step.State == "failed"
			if terminalStep {
				start := step
				start.State = "running"
				start.EndedAt = ""
				start.Usage = nil
				start.FinishReason = ""
				start.EndReason = ""
				start.EndMessage = ""
				mapped, err := m.applyStep(agentID, state, turn.TurnID, start, seq)
				if err != nil {
					return nil, err
				}
				events = append(events, mapped...)
			} else {
				mapped, err := m.applyStep(agentID, state, turn.TurnID, step, seq)
				if err != nil {
					return nil, err
				}
				events = append(events, mapped...)
			}
			for _, frame := range step.Frames {
				mapped, err := m.applyFrame(agentID, state, turn.TurnID, step.StepID, frame, seq)
				if err != nil {
					return nil, err
				}
				events = append(events, mapped...)
			}
			if terminalStep {
				mapped, err := m.applyStep(agentID, state, turn.TurnID, step, seq)
				if err != nil {
					return nil, err
				}
				events = append(events, mapped...)
			}
		}
		if terminal {
			mapped, err := m.applyTurn(agentID, state, turn, seq)
			if err != nil {
				return nil, err
			}
			events = append(events, mapped...)
		}
	}
	for _, prompt := range snapshot.Prompts {
		state.prompts[prompt.PromptID] = prompt
		mapped := m.applyPrompt(agentID, prompt, seq)
		events = append(events, mapped...)
	}
	return events, nil
}

func (m *transcriptEventMapper) applyOperation(agentID string, state *agentEventState, op transcriptOperation, seq int64) ([]Event, error) {
	switch op.Op {
	case "reset":
		if op.Snapshot == nil {
			return nil, fmt.Errorf("klient: transcript reset op missing snapshot agent=%s", agentID)
		}
		return m.applyResetOperation(agentID, *op.Snapshot, seq)
	case "turn.upsert":
		return m.applyTurn(agentID, state, op.Turn, seq)
	case "step.upsert":
		return m.applyStep(agentID, state, op.TurnID, op.Step, seq)
	case "frame.upsert":
		return m.applyFrame(agentID, state, op.TurnID, op.StepID, op.Frame, seq)
	case "append":
		return m.applyAppend(agentID, state, op, seq)
	case "marker.upsert":
		return m.applyMarker(agentID, op.Item, seq)
	case "prompt.upsert":
		state.prompts[op.Prompt.PromptID] = op.Prompt
		return m.applyPrompt(agentID, op.Prompt, seq), nil
	case "items.remove":
		m.removeItems(state, op.IDs)
		return nil, nil
	case "interaction.upsert", "attachment.upsert":
		return nil, nil
	default:
		return nil, fmt.Errorf("klient: unsupported transcript op %q agent=%s", op.Op, agentID)
	}
}

func (m *transcriptEventMapper) applyTurn(agentID string, state *agentEventState, turn TranscriptTurn, seq int64) ([]Event, error) {
	if turn.TurnID == "" {
		return nil, fmt.Errorf("klient: turn upsert missing turnId agent=%s", agentID)
	}
	current := state.turns[turn.TurnID]
	if current == nil {
		current = &turnEventState{}
		state.turns[turn.TurnID] = current
	}
	events := make([]Event, 0, 2)
	identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
	if turn.State == "running" && !current.started {
		current.started = true
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeTurnStarted, TurnStartedPayload{
			EventIdentity: identity,
			TurnID:        turn.TurnID,
			PromptID:      turn.TriggerPromptID,
			Origin:        turn.Origin,
			Prompt:        turn.Prompt,
		}))
	}
	if terminalTurnState(turn.State) && !current.ended {
		current.ended = true
		var eventErr *ErrorPayload
		if turn.Error != "" {
			eventErr = &ErrorPayload{Message: turn.Error}
		}
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeTurnEnded, TurnEndedPayload{
			EventIdentity: identity,
			TurnID:        turn.TurnID,
			PromptID:      turn.TriggerPromptID,
			Reason:        turn.State,
			Error:         eventErr,
			DurationMs:    turn.DurationMs,
		}))
	}
	current.turn = turn
	return events, nil
}

func (m *transcriptEventMapper) applyStep(agentID string, state *agentEventState, turnID string, step TranscriptStep, seq int64) ([]Event, error) {
	if turnID == "" {
		turnID = step.TurnID
	}
	if turnID == "" || step.StepID == "" {
		return nil, fmt.Errorf("klient: step upsert missing identity agent=%s", agentID)
	}
	key := stepKey(turnID, step.StepID)
	current := state.steps[key]
	if current == nil {
		current = &stepEventState{}
		state.steps[key] = current
	}
	identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
	events := make([]Event, 0, 2)
	if step.State == "running" && !current.started {
		current.started = true
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeTurnStepStarted, TurnStepPayload{
			EventIdentity: identity,
			TurnID:        turnID,
			Step:          step.Ordinal,
			StepID:        step.StepID,
		}))
	}
	if step.Retry != nil {
		retryKey := transcriptStepRetryKey(step.Retry)
		if retryKey != current.retryKey {
			current.retryKey = retryKey
			events = append(events, newEvent(m.sessionID, agentID, seq, TypeTurnStepRetrying, TurnStepRetryingPayload{
				EventIdentity: identity,
				TurnID:        turnID,
				Step:          step.Ordinal,
				StepID:        step.StepID,
				FailedAttempt: step.Retry.FailedAttempt,
				NextAttempt:   step.Retry.NextAttempt,
				MaxAttempts:   step.Retry.MaxAttempts,
				DelayMs:       step.Retry.DelayMs,
				ErrorName:     step.Retry.ErrorName,
				ErrorMessage:  step.Retry.ErrorMessage,
				StatusCode:    step.Retry.StatusCode,
			}))
		}
	}
	if step.State == "completed" || step.State == "interrupted" || step.State == "failed" {
		terminalKey := transcriptStepTerminalKey(step)
		if terminalKey != current.terminalKey {
			current.terminalKey = terminalKey
			typ := TypeTurnStepComplete
			if step.State != "completed" {
				typ = TypeTurnStepInterrupted
			}
			events = append(events, newEvent(m.sessionID, agentID, seq, typ, TurnStepCompletedPayload{
				EventIdentity: identity,
				TurnID:        turnID,
				Step:          step.Ordinal,
				StepID:        step.StepID,
				Usage:         step.Usage,
				FinishReason:  step.FinishReason,
				EndReason:     step.EndReason,
				EndMessage:    step.EndMessage,
			}))
		}
	}
	current.step = step
	return events, nil
}

func (m *transcriptEventMapper) applyFrame(agentID string, state *agentEventState, turnID, stepID string, frame TranscriptFrame, seq int64) ([]Event, error) {
	if turnID == "" || stepID == "" || frame.FrameID == "" {
		return nil, fmt.Errorf("klient: frame upsert missing identity agent=%s", agentID)
	}
	key := frameKey(turnID, stepID, frame.FrameID)
	current := state.frames[key]
	if current == nil {
		current = &frameEventState{turnID: turnID, stepID: stepID}
		state.frames[key] = current
	}
	previous := current.frame
	previousText := current.textUnits
	current.frame = frame
	switch frame.Kind {
	case "text", "thinking":
		nextText := transcriptFrameTextUnits(frame)
		_, novel := transcriptUnitsDiff(previousText, nextText)
		current.textUnits = nextText
		emission, pending := takeUTF16Emission(current.pendingText, novel)
		current.pendingText = pending
		if emission == "" {
			return nil, nil
		}
		identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
		if frame.Kind == "thinking" {
			return []Event{newEvent(m.sessionID, agentID, seq, TypeThinkingDelta, ThinkingDeltaPayload{
				EventIdentity: identity, TurnID: turnID, Delta: emission,
			})}, nil
		}
		if frame.Role == "user" {
			if frame.TaskID == "" {
				return nil, nil
			}
			return []Event{newEvent(m.sessionID, agentID, seq, TypeTaskNotified, struct {
				EventIdentity
				SourceID   string `json:"sourceId"`
				SourceKind string `json:"sourceKind,omitempty"`
				Text       string `json:"text,omitempty"`
			}{identity, frame.TaskID, "task", emission})}, nil
		}
		if frame.Role != "assistant" {
			return nil, nil
		}
		return []Event{newEvent(m.sessionID, agentID, seq, TypeAssistantDelta, AssistantDeltaPayload{
			EventIdentity: identity, TurnID: turnID, Delta: emission,
		})}, nil
	case "tool":
		return m.applyToolFrame(agentID, current, previous, frame, seq)
	case "notice":
		return m.applyNotice(agentID, frame, seq), nil
	default:
		return nil, fmt.Errorf("klient: unsupported transcript frame kind %q agent=%s", frame.Kind, agentID)
	}
}

func (m *transcriptEventMapper) applyToolFrame(agentID string, current *frameEventState, previous, frame TranscriptFrame, seq int64) ([]Event, error) {
	identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
	events := make([]Event, 0, 5)
	if len(frame.Input) > 0 && !current.started {
		current.started = true
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeToolCallStarted, ToolCallStartedPayload{
			EventIdentity: identity,
			TurnID:        current.turnID,
			ToolCallID:    frame.ToolCallID,
			Name:          frame.Name,
			Args:          frame.Input,
			Display:       frame.Display,
		}))
	}
	previousInput := current.inputTextUnits
	nextInput := transcriptFrameInputUnits(frame)
	_, novelInput := transcriptUnitsDiff(previousInput, nextInput)
	current.inputTextUnits = nextInput
	argumentsPart, pendingInput := takeUTF16Emission(current.pendingInput, novelInput)
	current.pendingInput = pendingInput
	if argumentsPart != "" {
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeToolCallDelta, ToolCallDeltaPayload{
			EventIdentity: identity,
			TurnID:        current.turnID,
			ToolCallID:    frame.ToolCallID,
			Name:          frame.Name,
			ArgumentsPart: argumentsPart,
		}))
	}
	if frame.Progress != nil {
		progressKeyBytes, _ := json.Marshal(frame.Progress)
		progressKey := string(progressKeyBytes)
		if progressKey != current.progressKey {
			current.progressKey = progressKey
			events = append(events, newEvent(m.sessionID, agentID, seq, TypeToolProgress, ToolProgressPayload{
				EventIdentity: identity,
				TurnID:        current.turnID,
				ToolCallID:    frame.ToolCallID,
				Update:        *frame.Progress,
			}))
		}
	}
	for i := len(previous.AgentRefs); i < len(frame.AgentRefs); i++ {
		ref := frame.AgentRefs[i]
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeSubagentSpawned, SubagentSpawnedPayload{
			EventIdentity:    identity,
			SubagentID:       ref.AgentID,
			ParentToolCallID: frame.ToolCallID,
			ParentAgentID:    agentID,
			Role:             ref.Role,
		}))
	}
	if (frame.State == "done" || frame.State == "error") && !current.resultEmitted {
		current.resultEmitted = true
		output := frame.Output
		if len(output) == 0 && frame.Error != "" {
			output, _ = json.Marshal(frame.Error)
		}
		events = append(events, newEvent(m.sessionID, agentID, seq, TypeToolResult, ToolResultPayload{
			EventIdentity: identity,
			TurnID:        current.turnID,
			ToolCallID:    frame.ToolCallID,
			Output:        output,
			IsError:       frame.State == "error",
		}))
	}
	return events, nil
}

func (m *transcriptEventMapper) applyAppend(agentID string, state *agentEventState, op transcriptOperation, seq int64) ([]Event, error) {
	target := op.Target
	offset := op.Offset
	key := frameKey(target.TurnID, target.StepID, target.FrameID)
	current := state.frames[key]
	if current == nil {
		return nil, &TranscriptGapError{AgentID: agentID, Expected: 0, Got: offset, Detail: "append target is unavailable"}
	}
	frame := current.frame
	if frame.Kind != "text" && frame.Kind != "thinking" {
		return nil, &TranscriptGapError{AgentID: agentID, Expected: 0, Got: offset, Detail: "append target is not text"}
	}
	merged, novel, err := appendTranscriptText(current.textUnits, offset, transcriptOperationTextUnits(op))
	if err != nil {
		return nil, &TranscriptGapError{AgentID: agentID, Expected: int64(len(current.textUnits)), Got: offset, Detail: err.Error()}
	}
	frame.textUnits = merged
	frame.Text = string(utf16.Decode(merged))
	current.textUnits = merged
	current.frame = frame
	emission, pending := takeUTF16Emission(current.pendingText, novel)
	current.pendingText = pending
	if emission == "" {
		return nil, nil
	}
	identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
	if frame.Kind == "thinking" {
		return []Event{newEvent(m.sessionID, agentID, seq, TypeThinkingDelta, ThinkingDeltaPayload{
			EventIdentity: identity, TurnID: target.TurnID, Delta: emission,
		})}, nil
	}
	if frame.Role == "user" {
		if frame.TaskID == "" {
			return nil, nil
		}
		return []Event{newEvent(m.sessionID, agentID, seq, TypeTaskNotified, struct {
			EventIdentity
			SourceID   string `json:"sourceId"`
			SourceKind string `json:"sourceKind,omitempty"`
			Text       string `json:"text,omitempty"`
		}{identity, frame.TaskID, "task", emission})}, nil
	}
	if frame.Role != "assistant" {
		return nil, nil
	}
	return []Event{newEvent(m.sessionID, agentID, seq, TypeAssistantDelta, AssistantDeltaPayload{
		EventIdentity: identity, TurnID: target.TurnID, Delta: emission,
	})}, nil
}

func (m *transcriptEventMapper) applyPrompt(agentID string, prompt TranscriptPrompt, seq int64) []Event {
	if !terminalPromptState(prompt.Status) {
		return nil
	}
	key := agentID + "\x00" + prompt.PromptID
	emitKey := prompt.Status + "\x00" + prompt.FinishedAt
	if m.promptEmits[key] == emitKey {
		return nil
	}
	m.promptEmits[key] = emitKey
	identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
	if prompt.Status == "aborted" {
		return []Event{newEvent(m.sessionID, agentID, seq, TypePromptAborted, PromptAbortedPayload{
			EventIdentity: identity,
			PromptID:      prompt.PromptID,
			AbortedAt:     prompt.FinishedAt,
		})}
	}
	return []Event{newEvent(m.sessionID, agentID, seq, TypePromptCompleted, PromptCompletedPayload{
		EventIdentity: identity,
		PromptID:      prompt.PromptID,
		FinishedAt:    prompt.FinishedAt,
		Reason:        prompt.Status,
	})}
}

func (m *transcriptEventMapper) applyMarker(agentID string, item TranscriptItem, seq int64) ([]Event, error) {
	if item.Kind != "marker" || item.Marker != "notice" {
		return nil, nil
	}
	var notice struct {
		Level   string          `json:"level"`
		Message string          `json:"message"`
		Event   json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(item.Payload, &notice); err != nil {
		return nil, fmt.Errorf("klient: decode notice marker agent=%s: %w", agentID, err)
	}
	frame := TranscriptFrame{Kind: "notice", Level: notice.Level, Message: notice.Message}
	event := m.applyNotice(agentID, frame, seq)
	if len(notice.Event) > 0 {
		for i := range event {
			if event[i].Type != TypeError {
				continue
			}
			var payload ErrorPayload
			if json.Unmarshal(notice.Event, &payload) == nil {
				payload.AgentID = agentID
				payload.SessionID = m.sessionID
				if payload.Message == "" {
					payload.Message = notice.Message
				}
				event[i] = newEvent(m.sessionID, agentID, seq, TypeError, payload)
			}
		}
	}
	return event, nil
}

func (m *transcriptEventMapper) applyNotice(agentID string, frame TranscriptFrame, seq int64) []Event {
	identity := EventIdentity{AgentID: agentID, SessionID: m.sessionID}
	switch frame.Level {
	case "error":
		return []Event{newEvent(m.sessionID, agentID, seq, TypeError, ErrorPayload{
			EventIdentity: identity,
			Code:          frame.Source,
			Message:       frame.Message,
		})}
	case "warning":
		return []Event{newEvent(m.sessionID, agentID, seq, TypeWarning, WarningPayload{
			EventIdentity: identity,
			Code:          frame.Source,
			Message:       frame.Message,
		})}
	default:
		return nil
	}
}

func (m *transcriptEventMapper) removeItems(state *agentEventState, ids []string) {
	for _, id := range ids {
		delete(state.turns, id)
		for key, step := range state.steps {
			if step.step.TurnID == id {
				delete(state.steps, key)
			}
		}
		for key, frame := range state.frames {
			if frame.turnID == id {
				delete(state.frames, key)
			}
		}
	}
}

func newEvent(sessionID, agentID string, seq int64, typ string, payload any) Event {
	data, err := json.Marshal(payload)
	if err != nil {
		typ = TypeProtocolError
		data, _ = json.Marshal(ProtocolErrorPayload{Msg: "klient: marshal event payload: " + err.Error(), Fatal: true})
	}
	return Event{Type: typ, Seq: seq, SessionID: sessionID, AgentID: agentID, Payload: data}
}

func transcriptTurnFromItem(item TranscriptItem) TranscriptTurn {
	return TranscriptTurn{
		Kind:            item.Kind,
		TurnID:          item.TurnID,
		TriggerPromptID: item.TriggerPromptID,
		Ordinal:         item.Ordinal,
		State:           item.State,
		Origin:          item.Origin,
		Prompt:          item.Prompt,
		AttachmentIDs:   item.AttachmentIDs,
		Steps:           item.Steps,
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
		Usage:           item.Usage,
		DurationMs:      item.DurationMs,
		Error:           item.Error,
		Wedged:          item.Wedged,
	}
}

func transcriptResetKey(seq int64, snapshot transcriptSnapshot) string {
	data, _ := json.Marshal(snapshot)
	return fmt.Sprintf("%d:%s", seq, data)
}

func transcriptStepTerminalKey(step TranscriptStep) string {
	data, _ := json.Marshal(struct {
		State        string        `json:"state"`
		Usage        *UsagePayload `json:"usage"`
		FinishReason string        `json:"finishReason"`
		EndReason    string        `json:"endReason"`
		EndMessage   string        `json:"endMessage"`
	}{step.State, step.Usage, step.FinishReason, step.EndReason, step.EndMessage})
	return string(data)
}

func transcriptStepRetryKey(retry *TranscriptStepRetry) string {
	return fmt.Sprintf("%d/%d/%d/%s", retry.FailedAttempt, retry.NextAttempt, retry.DelayMs, retry.ErrorMessage)
}

func terminalTurnState(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled"
}

func terminalPromptState(state string) bool {
	return state == "completed" || state == "failed" || state == "blocked" || state == "aborted"
}

func stepKey(turnID, stepID string) string {
	return turnID + "\x00" + stepID
}

func frameKey(turnID, stepID, frameID string) string {
	return turnID + "\x00" + stepID + "\x00" + frameID
}

func appendTranscriptText(localUnits []uint16, offset int64, chunkUnits []uint16) ([]uint16, []uint16, error) {
	if offset < 0 {
		return localUnits, nil, fmt.Errorf("negative append offset")
	}
	if offset > int64(len(localUnits)) {
		return localUnits, nil, fmt.Errorf("append offset beyond local text")
	}
	at := int(offset)
	if at+len(chunkUnits) <= len(localUnits) && equalUTF16(localUnits[at:at+len(chunkUnits)], chunkUnits) {
		return localUnits, nil, nil
	}
	overlap := len(localUnits) - at
	if overlap > len(chunkUnits) || !equalUTF16(localUnits[at:], chunkUnits[:overlap]) {
		return localUnits, nil, fmt.Errorf("append content does not match local suffix")
	}
	mergedUnits := append(append([]uint16{}, localUnits[:at]...), chunkUnits...)
	novel := cloneUTF16(chunkUnits[overlap:])
	return mergedUnits, novel, nil
}

func equalUTF16(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
