package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"

	"google.golang.org/genai"
)

func main() {
	// Parse CLI flags
	model := flag.String("model", "gemini-3-flash-preview", "Model to use")
	root := flag.String("root", "", "Project root (default: current working directory)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Resolve root path
	rootPath := *root
	if rootPath == "" {
		var err error
		rootPath, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving working directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Create sandbox
	sandbox, err := NewPathSandbox(rootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating sandbox: %v\n", err)
		os.Exit(1)
	}

	if *debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Project root: %s\n", sandbox.Root)
	}

	// Create Gemini client
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Gemini client: %v\n", err)
		os.Exit(1)
	}

	// Set up input reader
	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	// Create and run agent
	agent := NewAgent(client, getUserMessage, sandbox, *debug)
	agent.model = *model // Allow override via flag

	if err := agent.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error running agent: %v\n", err)
		os.Exit(1)
	}
}
