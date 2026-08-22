package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Limits bound every run. Zero fields fall back to DefaultLimits at run time.
type Limits struct {
	MaxModelTurns      int           // model calls per run
	MaxToolCalls       int           // tool invocations per run
	RunTimeout         time.Duration // wall-clock budget for one run
	ToolTimeout        time.Duration // wall-clock budget for one tool call
	MaxToolOutputBytes int64         // captured tool output per call
}

// DefaultLimits are the documented runtime defaults.
var DefaultLimits = Limits{
	MaxModelTurns:      32,
	MaxToolCalls:       64,
	RunTimeout:         10 * time.Minute,
	ToolTimeout:        60 * time.Second,
	MaxToolOutputBytes: 64 * 1024,
}

// Validate rejects non-positive limits.
func (l Limits) Validate() error {
	if l.MaxModelTurns <= 0 {
		return &ConfigError{Field: "max model turns", Err: errors.New("must be positive")}
	}
	if l.MaxToolCalls <= 0 {
		return &ConfigError{Field: "max tool calls", Err: errors.New("must be positive")}
	}
	if l.RunTimeout <= 0 {
		return &ConfigError{Field: "run timeout", Err: errors.New("must be positive")}
	}
	if l.ToolTimeout <= 0 {
		return &ConfigError{Field: "tool timeout", Err: errors.New("must be positive")}
	}
	if l.MaxToolOutputBytes <= 0 {
		return &ConfigError{Field: "max tool output bytes", Err: errors.New("must be positive")}
	}
	return nil
}

// WithDefaults returns l with zero fields replaced by DefaultLimits.
func (l Limits) WithDefaults() Limits {
	d := DefaultLimits
	if l.MaxModelTurns == 0 {
		l.MaxModelTurns = d.MaxModelTurns
	}
	if l.MaxToolCalls == 0 {
		l.MaxToolCalls = d.MaxToolCalls
	}
	if l.RunTimeout == 0 {
		l.RunTimeout = d.RunTimeout
	}
	if l.ToolTimeout == 0 {
		l.ToolTimeout = d.ToolTimeout
	}
	if l.MaxToolOutputBytes == 0 {
		l.MaxToolOutputBytes = d.MaxToolOutputBytes
	}
	return l
}

// ApplyEnv returns limits overridden by PINGU_MAX_MODEL_TURNS,
// PINGU_MAX_TOOL_CALLS, PINGU_RUN_TIMEOUT, PINGU_TOOL_TIMEOUT, and
// PINGU_MAX_TOOL_OUTPUT_BYTES. Invalid values are ConfigErrors.
func (l Limits) ApplyEnv() (Limits, error) {
	out := l
	if v := os.Getenv("PINGU_MAX_MODEL_TURNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return out, &ConfigError{Field: "PINGU_MAX_MODEL_TURNS", Err: fmt.Errorf("invalid value %q", v)}
		}
		out.MaxModelTurns = n
	}
	if v := os.Getenv("PINGU_MAX_TOOL_CALLS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return out, &ConfigError{Field: "PINGU_MAX_TOOL_CALLS", Err: fmt.Errorf("invalid value %q", v)}
		}
		out.MaxToolCalls = n
	}
	if v := os.Getenv("PINGU_RUN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return out, &ConfigError{Field: "PINGU_RUN_TIMEOUT", Err: fmt.Errorf("invalid duration %q", v)}
		}
		out.RunTimeout = d
	}
	if v := os.Getenv("PINGU_TOOL_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return out, &ConfigError{Field: "PINGU_TOOL_TIMEOUT", Err: fmt.Errorf("invalid duration %q", v)}
		}
		out.ToolTimeout = d
	}
	if v := os.Getenv("PINGU_MAX_TOOL_OUTPUT_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return out, &ConfigError{Field: "PINGU_MAX_TOOL_OUTPUT_BYTES", Err: fmt.Errorf("invalid value %q", v)}
		}
		out.MaxToolOutputBytes = n
	}
	return out, nil
}
