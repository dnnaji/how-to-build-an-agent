# Code Editing Agent — Unified Implementation Spec (1–5)

This document replaces `spec-1` through `spec-5` and defines a single implementable plan for:

1. Multi-tool calling in a single turn (no extra round-trips)
2. Streaming assistant output (fast perceived latency)
3. Modular Go file layout (maintainable codebase)
4. Robust path sandboxing (no filesystem escape, including symlinks)
5. Contextual tool errors + debug logging (LLM can recover)

## Goals
- Execute **all tool calls returned in one model message** and send **all tool responses back in one user message** (multi-part).
- Stream assistant text to the terminal while still supporting tool calling.
- Keep model history clean (one model message per turn, even when streaming).
- Prevent access outside a configured project root (including symlink escapes).
- Return structured tool results with actionable error messages and suggestions.

## Non-goals
- Implementing a full diff/patch tool or AST-based code editing.
- Network access tools or shell execution tools.
- A full test harness; this spec includes a manual test plan only.

## Current State (baseline)
`main.go` currently contains:
- CLI loop + Agent
- Tool declarations (`read_file`, `write_file`, `list_files`)
- Single-tool-call handling via `firstFunctionCall`
- Non-streaming `chat.SendMessage`
- Path restriction via string prefix check (not symlink-safe)
- Tool results as `map[string]any` with raw errors

## Proposed Architecture (Spec 3)
Split into focused files under `package main`:

- `main.go`
  - CLI flags (`--model`, `--root`, `--debug`)
  - terminal loop (stdin)
  - constructs `Agent` and calls `Run`

- `agent.go`
  - `type Agent struct { ... }`
  - conversation loop, streaming render, tool-call execution loop
  - owns `history []*genai.Content` (do not use `client.Chats` for streaming)

- `tools.go`
  - tool declarations (`[]*genai.Tool`)
  - `executeTool(ctx, call) ToolResult`
  - per-tool handlers (`readFile`, `writeFile`, `listFiles`)

- `sandbox.go` (or `paths.go`)
  - `type PathSandbox struct { Root string }`
  - safe path resolution for read/write/list (symlink-safe)

- `utils.go`
  - arg parsing helpers (e.g., `getStringArg`)
  - small helpers (sorting, suggestion matching, formatting)

## Implementation Order (recommended)
1. **Path sandboxing** (Spec 4) — safety-critical.
2. **Multi-tool collection + single multi-part response** (Spec 1) — correctness.
3. **Streaming with clean history** (Spec 2) — UX without corrupting history.
4. **Modularization** (Spec 3) — maintainability.
5. **Error envelopes + debug logging** (Spec 5) — quality.

## Spec 4 — Advanced Path Sandboxing (detailed)

### Requirements
- Define a single `RootPath` at startup (`--root`, default: `os.Getwd()`).
- All tool paths MUST be constrained to `RootPath`.
- Prevent escapes via:
  - `..` segments
  - absolute paths outside root
  - symlinks that point outside root (including symlinked parent dirs)
- Behavior must be well-defined when the target path does **not** exist (especially for `write_file`).

### Path Resolution API
Introduce:

- `type PathAccess int`
  - `AccessReadFile`
  - `AccessWriteFile`
  - `AccessListDir`

- `func NewPathSandbox(root string) (*PathSandbox, error)`
  - resolves `rootAbs := filepath.Abs(root)`
  - resolves `rootReal := filepath.EvalSymlinks(rootAbs)` (fail if it errors)
  - stores `Root = rootReal`

- `func (s *PathSandbox) Resolve(userPath string, access PathAccess) (string, error)`

### Resolution Rules
Given `userPath`:
1. Reject empty / whitespace-only paths (`invalid_argument`).
2. Normalize:
   - `clean := filepath.Clean(userPath)`
   - If you choose to disallow absolute paths, reject `filepath.IsAbs(clean)` with `invalid_argument`.
   - Otherwise allow absolute, but still validate it is under root.
3. Form the candidate:
   - If relative: `candidate := filepath.Join(s.Root, clean)`
   - If absolute: `candidate := clean`
   - `candidateAbs := filepath.Abs(candidate)`
4. Symlink protection:
   - For `AccessReadFile` and `AccessListDir`: `candidateReal := filepath.EvalSymlinks(candidateAbs)` (must succeed).
   - For `AccessWriteFile`:
     - If `EvalSymlinks(candidateAbs)` succeeds, use it (covers overwriting an existing file).
     - If it fails due to non-existence:
       - Evaluate the real path of the parent directory:
         - `parentReal := filepath.EvalSymlinks(filepath.Dir(candidateAbs))` (must succeed)
         - `candidateReal := filepath.Join(parentReal, filepath.Base(candidateAbs))`
       - This prevents writing through a symlinked directory that points outside root.
5. Root check:
   - `rel, err := filepath.Rel(s.Root, candidateReal)`
   - Reject if `rel == ".."` or `strings.HasPrefix(rel, ".."+string(filepath.Separator))`
6. Return `candidateReal`.

### Tool Schema Updates
Update tool descriptions so the model knows constraints:
- `read_file.path`: “Workspace-relative path under the project root.”
- `write_file.path`: “Workspace-relative path under the project root.”
- `list_files.path`: “Directory under the project root (use `.` for root).”

### Acceptance Criteria
- `read_file` cannot read `/etc/hosts` even if requested.
- `read_file` cannot read `./symlink_to_outside/secret.txt` if the symlink escapes root.
- `write_file` cannot write via a symlinked parent directory that escapes root.
- `list_files` cannot list outside root.

## Spec 1 — Multi-tool Calls in One Turn (detailed)

### Problem
Current logic only handles the **first** function call. If the model returns multiple tool calls, the agent ignores the rest or requires extra turns.

### Requirements
- Detect **all** tool calls in the model output for the current turn.
- Execute them and send back **one** user message containing **one FunctionResponse part per tool call**.
- Preserve call order (the i-th response maps to the i-th call).
- Support chained tool calls: after sending tool results, the model may return more tool calls; repeat until none remain.

### Extraction
Replace `firstFunctionCall` with:
- `func allFunctionCalls(res *genai.GenerateContentResponse) []*genai.FunctionCall`
  - reads `res.Candidates[0].Content.Parts`
  - returns every `part.FunctionCall != nil`

If using manual streaming history (recommended in Spec 2), also add:
- `func allFunctionCallsFromContent(c *genai.Content) []*genai.FunctionCall`

### Execution + response packaging
Implement:
- `func (a *Agent) executeToolCalls(ctx context.Context, calls []*genai.FunctionCall) []genai.Part`
  - sequential execution is acceptable (keep it simple and deterministic)
  - produce `[]genai.Part` where each element is:
    - `genai.Part{FunctionResponse: &genai.FunctionResponse{Name: call.Name, Response: toolResult.AsMap()}}`
  - send via one model call: `Send(...parts...)`

### Acceptance Criteria
- A prompt like “List files in A and B” results in one model call, two tool executions, one tool-response message, then the model’s combined answer.
- A response containing 2+ tool calls never drops any call.

## Spec 2 — Streaming Responses (detailed)

### Key constraint
`genai.Chat.SendMessageStream` appends each stream chunk as its own history entry. That bloats history and makes tool-calling turns messy.

### Requirement (clean history)
Use `client.Models.GenerateContentStream` with a manually-managed `history []*genai.Content` so the agent can:
- Stream to the terminal
- Merge chunks into a single `genai.Content{Role: "model"}` per turn
- Append exactly one model message per turn to history

### Streaming algorithm (one turn)
Given `history` and an input user content:
1. Append `userContent` to `history`.
2. Call `GenerateContentStream(ctx, model, history, config)` and range over chunks.
3. For each chunk:
   - Iterate `chunk.Candidates[0].Content.Parts` (if present).
   - If `part.Text != ""`: print it immediately (this is the incremental delta).
   - If `part.FunctionCall != nil`: collect it for later execution (do not print).
   - Also collect the `*genai.Content` from each chunk into `chunkContents`.
4. After stream ends:
   - Merge `chunkContents` into a single `modelContent` by concatenating all parts (role = `model`).
   - Append `modelContent` to `history`.
5. If any function calls were collected:
   - Execute Spec 1: create a user content containing all function responses, append to `history`, and start another model call (stream again).

### Terminal output rules
- Print `Gemini:` prefix once per assistant turn.
- Do not print any tool-call JSON; only show tool-call names separately (optional) as `→ read_file`.

### Acceptance Criteria
- Assistant text appears incrementally.
- History grows by exactly 2 contents per plain turn (user + model), and by 4+ for tool turns (user + model(function call) + user(function response) + model(final)).

## Spec 5 — Contextual Error Handling & Debug Logging (detailed)

### Tool result envelope
Use a consistent response shape for all tools:

- Success:
  - `{"ok": true, "data": {...}}`
- Failure:
  - `{"ok": false, "error": {"code": "not_found|invalid_argument|permission_denied|io_error", "message": "...", "suggestions": ["..."]}}`

Implement in Go:
- `type ToolResult struct { OK bool; Data map[string]any; Error *ToolError }`
- `type ToolError struct { Code string; Message string; Suggestions []string }`
- `func (r ToolResult) AsMap() map[string]any` (used for `FunctionResponse.Response`)

### Suggestion behavior
When a tool fails due to path issues:
- If the file/dir does not exist:
  - Suggest closest names from the parent directory (top 3).
  - Simple heuristic is fine:
    - case-insensitive exact prefix match first
    - then substring match
    - then edit-distance (optional) as last resort
- If the path escapes root:
  - Return `permission_denied` and suggest using a relative path under `.`.

### `--debug` flag
Add `--debug` that prints:
- every tool call name + args (as JSON)
- every tool response (as JSON)
- every sandbox resolution decision on error (root, cleaned path, rel)

Debug output should go to stderr so it doesn’t interleave with streamed assistant text as badly.

### Acceptance Criteria
- Missing file error includes at least one “Did you mean …” suggestion when possible.
- Debug mode clearly shows tool calls and responses in JSON.

## Manual Test Plan
From the `code-editing-agent` directory:

1. **Sandboxing**
   - `read_file` with `path: "../"` → denied
   - Create a symlink to `/etc` under root and attempt `read_file` through it → denied
   - `write_file` into a symlinked directory escaping root → denied

2. **Multi-tool**
   - Ask for two `read_file` calls in one message; verify both execute and one tool-response message is sent.

3. **Streaming**
   - Ask a long question; verify incremental output and a single `Gemini:` prefix.

4. **Error envelope**
   - Request a non-existent file; verify `ok=false` and suggestions.

