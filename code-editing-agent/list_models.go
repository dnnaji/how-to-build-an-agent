package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	for model, err := range client.Models.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\t%s\n", model.Name, model.DisplayName)
	}
}
