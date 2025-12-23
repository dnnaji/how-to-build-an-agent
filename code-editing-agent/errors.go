package main

// ToolError represents a structured error from a tool call.
type ToolError struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// ToolResult is the result envelope for all tool calls.
type ToolResult struct {
	OK    bool              `json:"ok"`
	Data  map[string]any    `json:"data,omitempty"`
	Error *ToolError        `json:"error,omitempty"`
}

// AsMap converts the ToolResult to a map for use in FunctionResponse.
func (r *ToolResult) AsMap() map[string]any {
	result := map[string]any{
		"ok": r.OK,
	}
	if r.Data != nil {
		result["data"] = r.Data
	}
	if r.Error != nil {
		result["error"] = map[string]any{
			"code":        r.Error.Code,
			"message":     r.Error.Message,
			"suggestions": r.Error.Suggestions,
		}
	}
	return result
}

// NewSuccessResult creates a successful tool result.
func NewSuccessResult(data map[string]any) *ToolResult {
	return &ToolResult{
		OK:   true,
		Data: data,
	}
}

// NewErrorResult creates a failed tool result.
func NewErrorResult(code, message string, suggestions []string) *ToolResult {
	return &ToolResult{
		OK: false,
		Error: &ToolError{
			Code:        code,
			Message:     message,
			Suggestions: suggestions,
		},
	}
}

// NewErrorResultFromSandbox converts a SandboxError to a ToolResult.
func NewErrorResultFromSandbox(err *SandboxError) *ToolResult {
	return NewErrorResult(err.Code, err.Message, err.Suggestions)
}
