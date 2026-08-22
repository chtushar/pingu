// Package runner executes the bounded model/tool loop for one run and emits
// an ordered, typed event stream. The terminal, future channels, tracing,
// and tests are all consumers of this stream.
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/chtushar/pingu/internal/config"
	"github.com/chtushar/pingu/internal/llm"
	"github.com/chtushar/pingu/internal/tools"
)

// EventKind enumerates run event kinds.
type EventKind string

const (
	EventRunStarted   EventKind = "run_started"
	EventTextDelta    EventKind = "text_delta"
	EventToolStarted  EventKind = "tool_started"
	EventToolFinished EventKind = "tool_finished"
	EventWarning      EventKind = "warning"
	EventError        EventKind = "error"
	EventRunFinished  EventKind = "run_finished"
)

// Event is one ordered run event.
type Event struct {
	Kind       EventKind
	Text       string    // text delta; warning or error message
	ToolCallID string    // tool events
	ToolName   string    // tool events
	Result     string    // tool output on EventToolFinished
	Turns      int       // EventRunFinished
	Usage      llm.Usage // EventRunFinished
	Err        error     // terminal error on EventRunFinished
}

// RunRequest describes one run.
type RunRequest struct {
	RunID        string
	Instructions string
	Model        string
	Input        string
	History      []llm.Message
	Tools        *tools.Registry // may be nil
}

// RunResult reports what the run appended to the conversation.
type RunResult struct {
	Messages []llm.Message // messages produced during this run
	Usage    llm.Usage
	Turns    int
}

// ErrLimitExhausted reports that a configured run limit was reached.
var ErrLimitExhausted = errors.New("run limit exhausted")

// Runner owns loop termination and ordering. It is safe for sequential use;
// concurrent runs require separate Runner values or external serialization.
type Runner struct {
	Provider llm.Provider
	Limits   config.Limits
}

type assembly struct {
	index   int
	id      string
	name    strings.Builder
	args    strings.Builder
	started bool
}

// Run executes the loop. emit is called synchronously and in order for every
// event; it must not block on the run. The returned error is nil only when
// the run completed normally. Cancellation propagates from ctx.
func (r *Runner) Run(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error) {
	limits := r.Limits.WithDefaults()
	if err := limits.Validate(); err != nil {
		return RunResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, limits.RunTimeout)
	defer cancel()

	var result RunResult
	var usage llm.Usage
	emit(Event{Kind: EventRunStarted, Text: req.RunID})
	finish := func(err error) (RunResult, error) {
		if err != nil {
			emit(Event{Kind: EventError, Text: err.Error()})
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = fmt.Errorf("%w: %v", err, ctxErr)
			}
		}
		result.Usage = usage
		emit(Event{Kind: EventRunFinished, Turns: result.Turns, Usage: usage, Err: err})
		return result, err
	}

	messages := make([]llm.Message, 0, len(req.History)+8)
	messages = append(messages, req.History...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: req.Input})

	var defs []llm.ToolDef
	if req.Tools != nil && !req.Tools.Empty() {
		for _, t := range req.Tools.List() {
			defs = append(defs, llm.ToolDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			})
		}
	}

	var toolCallsUsed int
	var syntheticID int

	for turn := 1; turn <= limits.MaxModelTurns; turn++ {
		result.Turns = turn
		stream, err := r.Provider.Stream(ctx, llm.Request{
			Model:    req.Model,
			System:   req.Instructions,
			Messages: messages,
			Tools:    defs,
		})
		if err != nil {
			return finish(fmt.Errorf("model call failed: %w", err))
		}

		var content strings.Builder
		calls := map[int]*assembly{}
		var order []int

		for {
			ev, err := stream.Next(ctx)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				stream.Close()
				return finish(fmt.Errorf("model stream failed: %w", err))
			}
			switch ev.Type {
			case llm.EventTextDelta:
				content.WriteString(ev.Text)
				emit(Event{Kind: EventTextDelta, Text: ev.Text})
			case llm.EventToolCallStart:
				c := calls[ev.ToolIndex]
				if c == nil {
					c = &assembly{index: ev.ToolIndex, id: ev.ToolCallID, started: true}
					calls[ev.ToolIndex] = c
					order = append(order, ev.ToolIndex)
				}
				if ev.ToolCallID != "" {
					c.id = ev.ToolCallID
				}
				c.name.WriteString(ev.ToolName)
			case llm.EventToolCallDelta:
				c := calls[ev.ToolIndex]
				if c == nil {
					c = &assembly{index: ev.ToolIndex, started: true}
					calls[ev.ToolIndex] = c
					order = append(order, ev.ToolIndex)
				}
				c.args.WriteString(ev.ArgumentsDelta)
			case llm.EventToolCallEnd:
				// Assembly is finalized when the message is assembled below.
			case llm.EventUsage:
				usage.InputTokens += ev.Usage.InputTokens
				usage.OutputTokens += ev.Usage.OutputTokens
			}
		}
		if err := stream.Close(); err != nil {
			slog.Debug("stream close failed", "error", err)
		}

		assistant := llm.Message{Role: llm.RoleAssistant, Content: content.String()}
		for _, idx := range order {
			c := calls[idx]
			if c.id == "" {
				syntheticID++
				c.id = fmt.Sprintf("call_%d", syntheticID)
			}
			assistant.ToolCalls = append(assistant.ToolCalls, llm.ToolCall{
				ID:        c.id,
				Name:      c.name.String(),
				Arguments: json.RawMessage(c.args.String()),
			})
		}
		messages = append(messages, assistant)
		result.Messages = append(result.Messages, assistant)

		if len(assistant.ToolCalls) == 0 {
			return finish(nil)
		}

		for _, call := range assistant.ToolCalls {
			toolCallsUsed++
			if toolCallsUsed > limits.MaxToolCalls {
				emit(Event{Kind: EventWarning, Text: "tool call budget exhausted"})
				return finish(fmt.Errorf("%w: max tool calls (%d)", ErrLimitExhausted, limits.MaxToolCalls))
			}
			emit(Event{Kind: EventToolStarted, ToolCallID: call.ID, ToolName: call.Name})

			out := r.executeTool(ctx, req.Tools, call, limits)
			if int64(len(out)) > limits.MaxToolOutputBytes {
				truncated := out[:limits.MaxToolOutputBytes]
				emit(Event{Kind: EventWarning, Text: fmt.Sprintf("tool %q output truncated to %d bytes", call.Name, limits.MaxToolOutputBytes)})
				out = truncated
			}
			emit(Event{Kind: EventToolFinished, ToolCallID: call.ID, ToolName: call.Name, Result: out})

			messages = append(messages, llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: out})
			result.Messages = append(result.Messages, llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: out})
		}
	}
	emit(Event{Kind: EventWarning, Text: "model turn budget exhausted"})
	return finish(fmt.Errorf("%w: max model turns (%d)", ErrLimitExhausted, limits.MaxModelTurns))
}

// executeTool runs one tool call and always returns a string result suitable
// for the conversation: tool errors become "error: ..." so the model can
// recover, matching the tool error convention.
func (r *Runner) executeTool(ctx context.Context, reg *tools.Registry, call llm.ToolCall, limits config.Limits) string {
	if reg == nil {
		return fmt.Sprintf("error: unknown tool %q", call.Name)
	}
	tool, ok := reg.Get(call.Name)
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", call.Name)
	}

	args := call.Arguments
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	if !json.Valid(args) {
		return fmt.Sprintf("error: invalid JSON arguments for tool %q", call.Name)
	}

	toolCtx, cancel := context.WithTimeout(ctx, limits.ToolTimeout)
	defer cancel()
	out, err := tool.Run(toolCtx, args)
	if err != nil {
		if ctxErr := toolCtx.Err(); ctxErr != nil && errors.Is(err, context.DeadlineExceeded) {
			return fmt.Sprintf("error: tool %q timed out after %s", call.Name, limits.ToolTimeout)
		}
		return "error: " + err.Error()
	}
	return out
}
