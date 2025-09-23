package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
)

type DiscordMessage struct {
	Content string `json:"content"`
}

type LambdaEvent struct {
	Message string `json:"message,omitempty"`
}

type LambdaResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

func handleRequest(ctx context.Context, event LambdaEvent) (LambdaResponse, error) {
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return LambdaResponse{
			StatusCode: 500,
			Body:       "Discord webhook URL not configured",
		}, fmt.Errorf("DISCORD_WEBHOOK_URL environment variable not set")
	}

	messageContent := "안녕하세요! Discord RSS 피드 봇 테스트 메시지입니다."
	if event.Message != "" {
		messageContent = event.Message
	}

	discordMsg := DiscordMessage{
		Content: messageContent,
	}

	jsonData, err := json.Marshal(discordMsg)
	if err != nil {
		return LambdaResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("Failed to marshal JSON: %v", err),
		}, err
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return LambdaResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("Failed to send Discord message: %v", err),
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LambdaResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("Discord API returned status: %d", resp.StatusCode),
		}, fmt.Errorf("error occurred: %d", resp.StatusCode)
	}

	return LambdaResponse{
		StatusCode: 200,
		Body:       "Discord message sent successfully",
	}, nil
}

func main() {
	lambda.Start(handleRequest)
}
