# Code Editing Agent — Implementation Summary

This is the full implementation of the unified spec (Specs 1–5) for the Code Editing Agent.

## Architecture

### File Organization

- **main.go** — CLI entry point, flag parsing (`--model`, `--root`, `--debug`), client setup
- **agent.go** — Core agent loop, streaming response handling, multi-tool execution
- **tools.go** — Tool declarations, per-tool handlers (`readFile`, `writeFile`, `listFiles`), tool execution
- **sandbox.go** — Path sandboxing with symlink safety, `PathSandbox` type
- **errors.go** — Structured error envelope, `ToolResult` and `ToolError` types
- **cmd_list_models.go** — Utility to list available Gemini models

## Features Implemented

### 1. Path Sandboxing (Spec 4)
- All filesystem access is restricted to a configurable project root
- Resolves symlinks to prevent escape attempts
- Separate handling for read, write, and list operations:
  - **Read/List**: Must evaluate symlinks successfully
  - **Write**: Allows overwriting existing files; for new files, validates parent dir
- Returns `SandboxError` with structured feedback and suggestions for near-matches

### 2. Multi-Tool Calling (Spec 1)
- Collects **all** function calls from a single model response
- Executes them sequentially
- Returns one user message with all `FunctionResponse` parts in correct order
- Supports chained calls: if model returns more tool calls after responses, the loop repeats

### 3. Streaming with Clean History (Spec 2)
- Uses `client.Models.GenerateContentStream` with manually-managed `history []*genai.Content`
- Merges all stream chunks into a single model message per turn
- Prints text incrementally to terminal
- Maintains clean history: 2 entries per plain turn, 4+ for tool-using turns
- Single `Gemini:` prefix per assistant turn

### 4. Modularization (Spec 3)
- Code split into focused modules by responsibility
- `package main` structure allows easy testing and refactoring
- Clear separation of concerns (CLI, agent loop, tools, sandboxing, errors)

### 5. Error Handling & Debug Logging (Spec 5)
- All tool results use `ToolResult` envelope: `{ok: bool, data: {...}, error: {...}}`
- `ToolError` includes `code` (not_found, invalid_argument, permission_denied, io_error), `message`, and `suggestions`
- Path resolution errors include "Did you mean…?" suggestions from parent directory
- `--debug` flag logs tool calls/responses and sandbox decisions to stderr

## Usage

```bash
# Build
go build -o agent .

# Run with defaults (current directory as root)
./agent

# Run with custom root
./agent --root /path/to/project

# Run with different model
./agent --model gemini-2.0-flash

# Enable debug logging
./agent --debug

# All options
./agent --root /path/to/project --model gemini-2.0-flash --debug
```

## Test Plan

### Sandboxing
```
read_file path: "../"              → denied (escape attempt)
read_file through symlink escaping → denied (symlink safety)
write_file via symlinked parent    → denied (write safety)
```

### Multi-tool
Request that naturally triggers 2 tools → both execute, one response message

### Streaming
Long prompt → incremental output, single `Gemini:` prefix, clean history

### Error Envelope
Request non-existent file → `ok: false`, error code, suggestions with near-matches

## Implementation Notes

- `genai.GenerateContentStream` returns an `iter.Seq2[*GenerateContentResponse, error]` (Go 1.22+)
- Parts are `[]*genai.Part` (pointers), not plain structs
- Content role must be exactly `"user"` or `"model"`
- Tool calls only work with streaming when history is manually managed
