package llm

import "encoding/json"

// Role represents the role of a message sender.
type Role string

const (
	RoleUser      Role = "user"
	RoleDeveloper Role = "developer"
	RoleAssistant Role = "assistant"
)

// InputItem represents a single item in the conversation input to the LLM.
type InputItem struct {
	Type string `json:"type"`

	// Message fields (type = "message")
	Role    Role   `json:"role,omitempty"`
	Content string `json:"content,omitempty"`

	// Function call fields (type = "function_call", round-tripped from output)
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// Function call output fields (type = "function_call_output")
	Output string `json:"output,omitempty"`

	// Raw stores provider-specific data for lossless round-tripping of
	// item types we don't model explicitly (reasoning, web_search, etc.).
	Raw json.RawMessage `json:"raw,omitempty"`
}

// NewMessage creates a message input item.
func NewMessage(content string, role Role) InputItem {
	return InputItem{Type: "message", Role: role, Content: content}
}

// NewFunctionCallOutput creates a function call output input item.
func NewFunctionCallOutput(callID, output string) InputItem {
	return InputItem{Type: "function_call_output", CallID: callID, Output: output}
}

// OutputItem represents a single item in the LLM's response output.
type OutputItem struct {
	Type string `json:"type"` // "message", "function_call", etc.

	// Message fields (type = "message")
	Content string `json:"content,omitempty"`

	// Function call fields (type = "function_call")
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// Raw stores provider-specific data for lossless round-tripping.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// UnmarshalJSON handles both the native format (content is a string) and the
// legacy OpenAI responses format (content is an array of content parts).
func (o *OutputItem) UnmarshalJSON(data []byte) error {
	// Use a shadow type to avoid infinite recursion.
	type shadow struct {
		Type      string          `json:"type"`
		Content   json.RawMessage `json:"content,omitempty"`
		CallID    string          `json:"call_id,omitempty"`
		Name      string          `json:"name,omitempty"`
		Arguments string          `json:"arguments,omitempty"`
		Raw       json.RawMessage `json:"raw,omitempty"`
	}
	var s shadow
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	o.Type = s.Type
	o.CallID = s.CallID
	o.Name = s.Name
	o.Arguments = s.Arguments
	o.Raw = s.Raw

	// Parse content: could be a string (native) or an array (legacy OpenAI).
	if len(s.Content) > 0 {
		// Try string first.
		var str string
		if err := json.Unmarshal(s.Content, &str); err == nil {
			o.Content = str
		} else {
			// Try legacy OpenAI array format: [{"type":"output_text","text":"..."}]
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(s.Content, &parts); err == nil {
				for _, p := range parts {
					if p.Type == "output_text" {
						o.Content += p.Text
					}
				}
			}
		}
	}

	return nil
}

// OutputToInput converts response output items into input items for the next
// LLM turn. Known types are mapped to structured InputItems; unknown types
// are preserved via Raw for provider-specific round-tripping.
func OutputToInput(output []OutputItem) []InputItem {
	items := make([]InputItem, 0, len(output))
	for _, o := range output {
		switch o.Type {
		case "message":
			items = append(items, InputItem{
				Type:    "message",
				Role:    RoleAssistant,
				Content: o.Content,
			})
		case "function_call":
			items = append(items, InputItem{
				Type:      "function_call",
				CallID:    o.CallID,
				Name:      o.Name,
				Arguments: o.Arguments,
			})
		default:
			// Preserve unknown types (reasoning, web_search, etc.) via Raw.
			items = append(items, InputItem{
				Type: o.Type,
				Raw:  o.Raw,
			})
		}
	}
	return items
}

// Response is the provider-agnostic LLM completion response.
type Response struct {
	Model        string       `json:"model"`
	Output       []OutputItem `json:"output"`
	InputTokens  int64        `json:"input_tokens"`
	OutputTokens int64        `json:"output_tokens"`
}

// UnmarshalJSON handles both the native format (flat input_tokens/output_tokens)
// and the legacy OpenAI responses format (nested usage object).
func (r *Response) UnmarshalJSON(data []byte) error {
	// Shadow type to avoid recursion — use a slice of json.RawMessage for
	// output so we can decode OutputItem ourselves with its custom unmarshaler.
	type shadow struct {
		Model        string            `json:"model"`
		Output       []json.RawMessage `json:"output"`
		InputTokens  int64             `json:"input_tokens"`
		OutputTokens int64             `json:"output_tokens"`
		// Legacy OpenAI format has nested usage.
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage,omitempty"`
	}
	var s shadow
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	r.Model = s.Model
	r.InputTokens = s.InputTokens
	r.OutputTokens = s.OutputTokens

	// Legacy OpenAI format stores tokens under "usage".
	if s.Usage != nil {
		if r.InputTokens == 0 {
			r.InputTokens = s.Usage.InputTokens
		}
		if r.OutputTokens == 0 {
			r.OutputTokens = s.Usage.OutputTokens
		}
	}

	r.Output = make([]OutputItem, 0, len(s.Output))
	for _, raw := range s.Output {
		var item OutputItem
		if err := json.Unmarshal(raw, &item); err != nil {
			continue // skip items we can't parse
		}
		r.Output = append(r.Output, item)
	}

	return nil
}

// ToolDefinition describes a tool the LLM can call.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}
