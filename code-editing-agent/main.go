package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/genai"
)

var tools = []*genai.Tool{
	{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "read_file",
				Description: "Read the contents of a file",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"path": {Type: genai.TypeString, Description: "Path to the file"},
					},
					Required: []string{"path"},
				},
			},
			{
				Name:        "write_file",
				Description: "Write content to a file",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"path":    {Type: genai.TypeString, Description: "Path to the file"},
						"content": {Type: genai.TypeString, Description: "Content to write"},
					},
					Required: []string{"path", "content"},
				},
			},
			{
				Name:        "list_files",
				Description: "List files in a directory",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"path": {Type: genai.TypeString, Description: "Directory path"},
					},
					Required: []string{"path"},
				},
			},
		},
	},
}

func main() {

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err.Error())
		return
	}

	scanner := bufio.NewScanner(os.Stdin)

	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	baseDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error resolving working directory: %v\n", err.Error())
		return
	}

	agent := NewAgent(client, getUserMessage, baseDir)
	err = agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error running agent: %v\n", err.Error())
	}
}

func NewAgent(client *genai.Client, getUserMessage func() (string, bool), baseDir string) *Agent {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		absBase = baseDir
	}
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
		baseDir:        absBase,
	}
}

type Agent struct {
	client         *genai.Client
	getUserMessage func() (string, bool)
	baseDir        string
}

func (a *Agent) Run(ctx context.Context) error {
	model := "gemini-3-flash-preview"
	fmt.Printf("Chat with %s (use 'ctlr-c' to exit)\n", model)

	config := &genai.GenerateContentConfig{
		Tools: tools,
	}
	chat, err := a.client.Chats.Create(ctx, model, config, nil)
	if err != nil {
		return err
	}

	for {
		fmt.Print("\u001b[94mYou:\u001b[0m ")
		userInput, ok := a.getUserMessage()
		if !ok {
			break
		}

		result, err := chat.SendMessage(ctx, genai.Part{Text: userInput})
		if err != nil {
			return err
		}

		// Handle tool calls (including chained calls).
		for {
			fc := firstFunctionCall(result)
			if fc == nil {
				break
			}
			fmt.Printf("\u001b[92m→ %s\u001b[0m\n", fc.Name)
			resp := a.executeTool(fc)
			result, err = chat.SendMessage(ctx, genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: resp,
				},
			})
			if err != nil {
				return err
			}
		}

		fmt.Printf("\u001b[93mGemini:\u001b[0m %s\n", result.Text())
	}
	return nil
}

func (a *Agent) executeTool(fc *genai.FunctionCall) map[string]any {
	switch fc.Name {
	case "read_file":
		path, err := getStringArg(fc, "path")
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		path, err = a.resolvePath(path)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"content": string(content)}

	case "write_file":
		path, err := getStringArg(fc, "path")
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		content, err := getStringArg(fc, "content")
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		path, err = a.resolvePath(path)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		err = os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"success": true, "message": fmt.Sprintf("Wrote to %s", path)}

	case "list_files":
		path, err := getStringArg(fc, "path")
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		path, err = a.resolvePath(path)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		var files []string
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			files = append(files, name)
		}
		return map[string]any{"files": files}

	default:
		return map[string]any{"error": fmt.Sprintf("Unknown tool: %s", fc.Name)}
	}
}

func firstFunctionCall(result *genai.GenerateContentResponse) *genai.FunctionCall {
	if result == nil || result.Candidates == nil || len(result.Candidates) == 0 {
		return nil
	}
	for _, part := range result.Candidates[0].Content.Parts {
		if fc := part.FunctionCall; fc != nil {
			return fc
		}
	}
	return nil
}

func getStringArg(fc *genai.FunctionCall, key string) (string, error) {
	raw, ok := fc.Args[key]
	if !ok {
		return "", fmt.Errorf("missing argument: %s", key)
	}
	val, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("argument %s must be a string", key)
	}
	if strings.TrimSpace(val) == "" {
		return "", fmt.Errorf("argument %s cannot be empty", key)
	}
	return val, nil
}

func (a *Agent) resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		path = filepath.Clean(path)
	} else {
		path = filepath.Join(a.baseDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	base := a.baseDir
	if !strings.HasSuffix(base, string(os.PathSeparator)) {
		base += string(os.PathSeparator)
	}
	if abs != a.baseDir && !strings.HasPrefix(abs, base) {
		return "", fmt.Errorf("path escapes base directory")
	}
	return abs, nil
}
