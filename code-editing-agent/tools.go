package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"

	"google.golang.org/genai"
)

// getTools returns the tool definitions for the agent.
func getTools() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "read_file",
					Description: "Read the contents of a file. Workspace-relative path under the project root.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "Workspace-relative path under the project root.",
							},
						},
						Required: []string{"path"},
					},
				},
				{
					Name:        "write_file",
					Description: "Write content to a file. Workspace-relative path under the project root.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "Workspace-relative path under the project root.",
							},
							"content": {
								Type:        genai.TypeString,
								Description: "Content to write to the file.",
							},
						},
						Required: []string{"path", "content"},
					},
				},
				{
					Name:        "list_files",
					Description: "List files in a directory. Use '.' for the project root.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "Directory under the project root (use '.' for root).",
							},
						},
						Required: []string{"path"},
					},
				},
				{
					Name:        "get_weather",
					Description: "Get the current weather for a given location (e.g., '[REDACTED]' or 'Houston, TX').",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"location": {
								Type:        genai.TypeString,
								Description: "The city and state, or zip code.",
							},
						},
						Required: []string{"location"},
					},
				},
			},
		},
	}
}

// executeTool executes a function call and returns a ToolResult.
func executeTool(fc *genai.FunctionCall, sandbox *PathSandbox, debugMode bool) *ToolResult {
	if debugMode {
		fmt.Fprintf(os.Stderr, "[DEBUG] Tool call: %s with args: %v\n", fc.Name, fc.Args)
	}

	var result *ToolResult

	switch fc.Name {
	case "read_file":
		result = readFile(fc, sandbox)
	case "write_file":
		result = writeFile(fc, sandbox)
	case "list_files":
		result = listFiles(fc, sandbox)
	case "get_weather":
		result = getWeather(fc, sandbox)
	default:
		result = NewErrorResult("invalid_argument", fmt.Sprintf("unknown tool: %s", fc.Name), nil)
	}

	if debugMode {
		fmt.Fprintf(os.Stderr, "[DEBUG] Tool response: %v\n", result.AsMap())
	}

	return result
}

// readFile reads and returns file contents.
func readFile(fc *genai.FunctionCall, sandbox *PathSandbox) *ToolResult {
	path, err := getStringArg(fc, "path")
	if err != nil {
		return NewErrorResult("invalid_argument", err.Error(), nil)
	}

	resolvedPath, err := sandbox.Resolve(path, AccessReadFile)
	if sandboxErr, ok := err.(*SandboxError); ok {
		return NewErrorResultFromSandbox(sandboxErr)
	}
	if err != nil {
		return NewErrorResult("io_error", fmt.Sprintf("failed to resolve path: %v", err), nil)
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return NewErrorResult("io_error", fmt.Sprintf("failed to read file: %v", err), nil)
	}

	return NewSuccessResult(map[string]any{
		"content": string(content),
	})
}

// writeFile writes content to a file.
func writeFile(fc *genai.FunctionCall, sandbox *PathSandbox) *ToolResult {
	path, err := getStringArg(fc, "path")
	if err != nil {
		return NewErrorResult("invalid_argument", err.Error(), nil)
	}

	content, err := getStringArg(fc, "content")
	if err != nil {
		return NewErrorResult("invalid_argument", err.Error(), nil)
	}

	resolvedPath, err := sandbox.Resolve(path, AccessWriteFile)
	if sandboxErr, ok := err.(*SandboxError); ok {
		return NewErrorResultFromSandbox(sandboxErr)
	}
	if err != nil {
		return NewErrorResult("io_error", fmt.Sprintf("failed to resolve path: %v", err), nil)
	}

	err = os.WriteFile(resolvedPath, []byte(content), 0644)
	if err != nil {
		return NewErrorResult("io_error", fmt.Sprintf("failed to write file: %v", err), nil)
	}

	return NewSuccessResult(map[string]any{
		"message": fmt.Sprintf("wrote %d bytes to %s", len(content), path),
	})
}

// listFiles lists the contents of a directory.
func listFiles(fc *genai.FunctionCall, sandbox *PathSandbox) *ToolResult {
	path, err := getStringArg(fc, "path")
	if err != nil {
		return NewErrorResult("invalid_argument", err.Error(), nil)
	}

	resolvedPath, err := sandbox.Resolve(path, AccessListDir)
	if sandboxErr, ok := err.(*SandboxError); ok {
		return NewErrorResultFromSandbox(sandboxErr)
	}
	if err != nil {
		return NewErrorResult("io_error", fmt.Sprintf("failed to resolve path: %v", err), nil)
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return NewErrorResult("io_error", fmt.Sprintf("failed to list directory: %v", err), nil)
	}

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}

	// Sort for consistent output
	sort.Strings(files)

	return NewSuccessResult(map[string]any{
		"files": files,
	})
}

// getWeather fetches the weather for a location.
func getWeather(fc *genai.FunctionCall, sandbox *PathSandbox) *ToolResult {
	location, err := getStringArg(fc, "location")
	if err != nil {
		return NewErrorResult("invalid_argument", err.Error(), nil)
	}

	// This is a simplified implementation using Open-Meteo.
	// In a real-world scenario, you would first geocode the location to lat/long.
	// For this tool, we'll use a hardcoded lookup for common zip codes or just use a default for demo.
	
	lat, lon := "[REDACTED]", "[REDACTED]" // Coordinates for [REDACTED], TX ([REDACTED])
	if location != "[REDACTED]" {
		// In a real tool, we would call a geocoding API here.
		// For now, let's just use these coordinates as a placeholder if not [REDACTED].
	}

	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current_weather=true", lat, lon)
	resp, err := http.Get(url)
	if err != nil {
		return NewErrorResult("network_error", fmt.Sprintf("failed to fetch weather: %v", err), nil)
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return NewErrorResult("parse_error", fmt.Sprintf("failed to parse weather response: %v", err), nil)
	}

	return NewSuccessResult(data)
}

// getStringArg retrieves a string argument from a function call.
func getStringArg(fc *genai.FunctionCall, key string) (string, error) {
	raw, ok := fc.Args[key]
	if !ok {
		return "", fmt.Errorf("missing argument: %s", key)
	}
	val, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("argument %s must be a string", key)
	}
	return val, nil
}
