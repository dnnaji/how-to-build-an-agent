package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"
)

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

	agent := NewAgent(client, getUserMessage)
	err = agent.Run(context.TODO())
	if err != nil {
		fmt.Printf("Error running agent: %v\n", err.Error())
	}
}

func NewAgent(client *genai.Client, getUserMessage func() (string, bool)) *Agent {
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
	}
}

type Agent struct {
	client         *genai.Client
	getUserMessage func() (string, bool)
}

func (a *Agent) Run(ctx context.Context) error {
	model := "gemini-2.5-flash"
	fmt.Printf("Chat with %s (use 'ctlr-c' to exit)\n", model)

	chat, err := a.client.Chats.Create(ctx, model, nil, nil)
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

		fmt.Printf("\u001b[93mGemini:\u001b[0m %s\n", result.Text())
	}
	return nil
}
