package main

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// Agent manages the conversation and tool execution.
type Agent struct {
	client         *genai.Client
	getUserMessage func() (string, bool)
	sandbox        *PathSandbox
	history        []*genai.Content
	model          string
	config         *genai.GenerateContentConfig
	debugMode      bool
}

// NewAgent creates a new Agent.
func NewAgent(client *genai.Client, getUserMessage func() (string, bool), sandbox *PathSandbox, debugMode bool) *Agent {
	model := "gemini-2.0-flash"
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
		sandbox:        sandbox,
		history:        []*genai.Content{},
		model:          model,
		config: &genai.GenerateContentConfig{
			Tools: getTools(),
		},
		debugMode: debugMode,
	}
}

// Run starts the main agent loop.
func (a *Agent) Run(ctx context.Context) error {
	fmt.Printf("Chat with %s (use ctrl-c to exit)\n", a.model)

	for {
		fmt.Print("\033[94mYou:\033[0m ")
		userInput, ok := a.getUserMessage()
		if !ok {
			break
		}

		// Append user message to history
		userContent := &genai.Content{
			Role: "user",
			Parts: []*genai.Part{
				{Text: userInput},
			},
		}
		a.history = append(a.history, userContent)

		// Stream and handle function calls
		if err := a.processStreamWithTools(ctx); err != nil {
			return err
		}
	}

	return nil
}

// processStreamWithTools handles a single turn of streaming + tool calls.
// It repeats until no more function calls are returned.
func (a *Agent) processStreamWithTools(ctx context.Context) error {
	for {
		// Stream the model response
		modelContent, calls, err := a.streamModelResponse(ctx)
		if err != nil {
			return err
		}

		// Append the model response to history
		a.history = append(a.history, modelContent)

		// If no tool calls, we're done with this turn
		if len(calls) == 0 {
			break
		}

		// Execute all tool calls and collect responses
		toolResponseParts := a.executeToolCalls(calls)

		// Create a user message containing all function responses
		toolResponseContent := &genai.Content{
			Role:  "user",
			Parts: toolResponseParts,
		}
		a.history = append(a.history, toolResponseContent)

		// Continue the loop to stream the next model response
	}

	return nil
}

// streamModelResponse streams the model response and returns the merged content + any function calls.
func (a *Agent) streamModelResponse(ctx context.Context) (*genai.Content, []*genai.FunctionCall, error) {
	stream := a.client.Models.GenerateContentStream(ctx, a.model, a.history, a.config)

	var allParts []*genai.Part
	var allCalls []*genai.FunctionCall
	firstOutput := true

	for resp, err := range stream {
		if err != nil {
			return nil, nil, fmt.Errorf("stream error: %w", err)
		}

		if resp == nil || len(resp.Candidates) == 0 {
			continue
		}

		for _, part := range resp.Candidates[0].Content.Parts {
			// Handle text: print immediately
			if part.Text != "" {
				if firstOutput {
					fmt.Print("\033[93mGemini:\033[0m ")
					firstOutput = false
				}
				fmt.Print(part.Text)
			}

			// Handle function calls: collect for later
			if part.FunctionCall != nil {
				allCalls = append(allCalls, part.FunctionCall)
			}

			// Add to parts for history
			allParts = append(allParts, part)
		}
	}

	if !firstOutput {
		fmt.Println() // Newline after streaming text
	}

	// Merge all parts into a single model content
	modelContent := &genai.Content{
		Role:  "model",
		Parts: allParts,
	}

	return modelContent, allCalls, nil
}

// executeToolCalls executes all function calls and returns FunctionResponse parts.
func (a *Agent) executeToolCalls(calls []*genai.FunctionCall) []*genai.Part {
	parts := make([]*genai.Part, len(calls))

	for i, call := range calls {
		fmt.Printf("\033[92m→ %s\033[0m\n", call.Name)

		result := executeTool(call, a.sandbox, a.debugMode)

		parts[i] = &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     call.Name,
				Response: result.AsMap(),
			},
		}
	}

	return parts
}
