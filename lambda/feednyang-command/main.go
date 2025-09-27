package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/mmcdole/gofeed"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Feed struct {
	BlogName       string    `bson:"blogName" json:"blogName"`
	RssURL         string    `bson:"rssUrl" json:"rssUrl"`
	AddedAt        time.Time `bson:"addedAt" json:"addedAt"`
	LastPostLink   string    `bson:"lastPostLink" json:"lastPostLink"`
	TotalPostsSent int       `bson:"totalPostsSent" json:"totalPostsSent"`
}

type DiscordChannel struct {
	ID        string    `bson:"_id" json:"_id"`
	Feeds     []Feed    `bson:"feeds" json:"feeds"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

type DiscordInteraction struct {
	Type int                    `json:"type"`
	Data DiscordInteractionData `json:"data"`
	ID   string                 `json:"id"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
	Token     string `json:"token"`
}

type DiscordInteractionData struct {
	ID      string                         `json:"id"`
	Name    string                         `json:"name"`
	Type    int                            `json:"type"`
	Options []DiscordInteractionDataOption `json:"options"`
}

type DiscordInteractionDataOption struct {
	Name  string `json:"name"`
	Type  int    `json:"type"`
	Value any    `json:"value"`
}

type DiscordInteractionResponse struct {
	Type int                            `json:"type"`
	Data DiscordInteractionResponseData `json:"data"`
}

type DiscordInteractionResponseData struct {
	Content string `json:"content"`
	Flags   int    `json:"flags,omitempty"`
}

const (
	InteractionTypePing                = 1
	InteractionTypeApplicationCommand  = 2
	ResponseTypePong                   = 1
	ResponseTypeChannelMessage         = 4
	ResponseTypeDeferredChannelMessage = 5
	MessageFlagEphemeral               = 64

	AlreadyRegisteredFeed             = "⚠️ 이미 등록된 피드다냥"
	FeedNotFound                      = "❌ 피드 못 찾겠다냥..."
	FeedSuccessfullyAdded             = "✅ 피드가 성공적으로 추가되었다냥~!"
	FeedSuccessfullyDeleted           = "✅ 피드가 성공적으로 삭제되었다냥~!"
	ErrorOccurredOnAddFeed            = "❌ 피드 추가에 실패했다냥..."
	ErrorOccurredOnDatabaseConnection = "❌ 데이터베이스 연결 오류다냥..."
	ErrorOccurredOnDeleteFeed         = "❌ 피드 삭제에 실패했다냥..."
	ErrorOccurredOnFeedParsing        = "❌ 피드 조회 중 오류가 발생했다냥~"
	InvalidRSSFeed                    = "❌ RSS 피드가 유효하지 않다냥!"
	NoRegisteredFeed                  = "⚠️ 이 채널에 등록된 피드가 없다냥~"
	ShouldInputRssUrl                 = "❌ RSS URL을 입력하라냥!"
	ShouldInputFeed                   = "❌ 삭제할 피드를 입력하라냥! (번호 / 블로그 제목 / URL)"
	UnknownCommand                    = "❌ 뭔 말이냥..."
	HelpMessage                       = "📚 **피드냥 명령어 도움말** 📚\n\n" +
		"🔸 `/add <RSS_URL>` - RSS 피드를 추가하라냥!\n" +
		"🔸 `/list` - 등록된 피드 목록을 확인하라냥!\n" +
		"🔸 `/remove <번호|이름|URL>` - 피드를 삭제하라냥!\n" +
		"🔸 `/help` - 이 도움말을 보여준다냥!\n\n" +
		"💡 **사용 예시:**\n" +
		"• `/add https://example.com/rss`\n" +
		"• `/remove 1` 또는 `/remove 블로그이름`\n\n" +
		"🚀 **피드냥**은 기술 블로그 RSS 피드를 관리해주는 봇이다냥~!"
)

func verifyDiscordSignature(signature, timestamp, body, publicKey string) bool {
	sig, err := hex.DecodeString(signature)
	if err != nil {
		log.Printf("Failed to decode signature: %v", err)
		return false
	}

	pub, err := hex.DecodeString(publicKey)
	if err != nil {
		log.Printf("Failed to decode public key: %v", err)
		return false
	}

	message := timestamp + body
	return ed25519.Verify(pub, []byte(message), sig)
}

func connectMongoDB(ctx context.Context) (*mongo.Client, error) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return nil, fmt.Errorf("MONGODB_URI environment variable not set")
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	return client, nil
}

func validateRSSFeed(url string) (*gofeed.Feed, error) {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	fp := gofeed.NewParser()
	fp.Client = httpClient
	fp.UserAgent = "Mozilla/5.0 (compatible; FeedNyang/1.0; +https://github.com/nmin11/feednyang)"

	feed, err := fp.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid RSS feed: %v", err)
	}

	if feed.Title == "" {
		return nil, fmt.Errorf("RSS feed has no title")
	}

	return feed, nil
}

func handleListCommand(ctx context.Context, channelID string) DiscordInteractionResponse {
	client, err := connectMongoDB(ctx)
	if err != nil {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnDatabaseConnection,
				Flags:   MessageFlagEphemeral,
			},
		}
	}
	defer client.Disconnect(ctx)

	channelCollection := client.Database("feednyang").Collection("discord_channels")
	var channel DiscordChannel

	err = channelCollection.FindOne(ctx, bson.M{"_id": channelID}).Decode(&channel)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return DiscordInteractionResponse{
				Type: ResponseTypeChannelMessage,
				Data: DiscordInteractionResponseData{
					Content: NoRegisteredFeed,
				},
			}
		}
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnFeedParsing,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	if len(channel.Feeds) == 0 {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: NoRegisteredFeed,
			},
		}
	}

	content := "📋 **등록된 피드 목록:**\n\n"
	for i, feed := range channel.Feeds {
		content += fmt.Sprintf("%d. **%s**\n📎 %s\n📊 전송된 포스트: %d개\n\n",
			i+1, feed.BlogName, feed.RssURL, feed.TotalPostsSent)
	}

	return DiscordInteractionResponse{
		Type: ResponseTypeChannelMessage,
		Data: DiscordInteractionResponseData{
			Content: content,
		},
	}
}

func handleAddCommand(ctx context.Context, channelID string, feedURL string) DiscordInteractionResponse {
	feed, err := validateRSSFeed(feedURL)
	if err != nil {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: InvalidRSSFeed,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	client, err := connectMongoDB(ctx)
	if err != nil {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnDatabaseConnection,
				Flags:   MessageFlagEphemeral,
			},
		}
	}
	defer client.Disconnect(ctx)

	channelCollection := client.Database("feednyang").Collection("discord_channels")
	var channel DiscordChannel

	err = channelCollection.FindOne(ctx, bson.M{"_id": channelID}).Decode(&channel)
	if err != nil && err != mongo.ErrNoDocuments {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnDatabaseConnection,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	for _, existingFeed := range channel.Feeds {
		if existingFeed.RssURL == feedURL {
			return DiscordInteractionResponse{
				Type: ResponseTypeChannelMessage,
				Data: DiscordInteractionResponseData{
					Content: fmt.Sprintf("%s: **%s**", AlreadyRegisteredFeed, existingFeed.BlogName),
					Flags:   MessageFlagEphemeral,
				},
			}
		}
	}

	newFeed := Feed{
		BlogName:       feed.Title,
		RssURL:         feedURL,
		AddedAt:        time.Now(),
		LastPostLink:   "",
		TotalPostsSent: 0,
	}

	if err == mongo.ErrNoDocuments {
		channel = DiscordChannel{
			ID:        channelID,
			Feeds:     []Feed{newFeed},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_, err = channelCollection.InsertOne(ctx, channel)
	} else {
		channel.Feeds = append(channel.Feeds, newFeed)
		channel.UpdatedAt = time.Now()
		_, err = channelCollection.ReplaceOne(ctx, bson.M{"_id": channelID}, channel)
	}

	if err != nil {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnAddFeed,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	return DiscordInteractionResponse{
		Type: ResponseTypeChannelMessage,
		Data: DiscordInteractionResponseData{
			Content: fmt.Sprintf("%s\n**%s**\n📎 %s", FeedSuccessfullyAdded, feed.Title, feedURL),
		},
	}
}

func handleHelpCommand() DiscordInteractionResponse {
	return DiscordInteractionResponse{
		Type: ResponseTypeChannelMessage,
		Data: DiscordInteractionResponseData{
			Content: HelpMessage,
		},
	}
}

func handleRemoveCommand(ctx context.Context, channelID string, feedIdentifier string) DiscordInteractionResponse {
	client, err := connectMongoDB(ctx)
	if err != nil {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnDatabaseConnection,
				Flags:   MessageFlagEphemeral,
			},
		}
	}
	defer client.Disconnect(ctx)

	channelCollection := client.Database("feednyang").Collection("discord_channels")
	var channel DiscordChannel

	err = channelCollection.FindOne(ctx, bson.M{"_id": channelID}).Decode(&channel)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return DiscordInteractionResponse{
				Type: ResponseTypeChannelMessage,
				Data: DiscordInteractionResponseData{
					Content: NoRegisteredFeed,
					Flags:   MessageFlagEphemeral,
				},
			}
		}
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnDatabaseConnection,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	index := -1
	if idx, err := strconv.Atoi(feedIdentifier); err == nil && idx > 0 && idx <= len(channel.Feeds) {
		index = idx - 1
	} else {
		normalizedInput := strings.ToLower(strings.ReplaceAll(feedIdentifier, " ", ""))
		for i, feed := range channel.Feeds {
			normalizedBlogName := strings.ToLower(strings.ReplaceAll(feed.BlogName, " ", ""))
			if normalizedBlogName == normalizedInput || feed.RssURL == feedIdentifier {
				index = i
				break
			}
		}
	}

	if index == -1 {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: fmt.Sprintf("%s **%s**\n`/list` 명령어로 피드 번호 / 이름 / URL 을 확인하라냥!", FeedNotFound, feedIdentifier),
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	removedFeed := channel.Feeds[index]
	channel.Feeds = append(channel.Feeds[:index], channel.Feeds[index+1:]...)
	channel.UpdatedAt = time.Now()

	_, err = channelCollection.ReplaceOne(ctx, bson.M{"_id": channelID}, channel)
	if err != nil {
		return DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: ErrorOccurredOnDeleteFeed,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	return DiscordInteractionResponse{
		Type: ResponseTypeChannelMessage,
		Data: DiscordInteractionResponseData{
			Content: fmt.Sprintf("%s **%s**", FeedSuccessfullyDeleted, removedFeed.BlogName),
		},
	}
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	publicKey := os.Getenv("DISCORD_PUBLIC_KEY")
	if publicKey != "" {
		signature := request.Headers["x-signature-ed25519"]
		timestamp := request.Headers["x-signature-timestamp"]

		if signature == "" || timestamp == "" {
			log.Printf("Missing Discord signature headers")
			return events.APIGatewayProxyResponse{
				StatusCode: 401,
				Body:       "Unauthorized",
			}, nil
		}

		if !verifyDiscordSignature(signature, timestamp, request.Body, publicKey) {
			log.Printf("Discord signature verification failed")
			return events.APIGatewayProxyResponse{
				StatusCode: 401,
				Body:       "Unauthorized",
			}, nil
		}
	}

	var interaction DiscordInteraction
	if err := json.Unmarshal([]byte(request.Body), &interaction); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       "Invalid JSON",
		}, nil
	}

	if interaction.Type == InteractionTypePing {
		response := DiscordInteractionResponse{
			Type: ResponseTypePong,
		}
		responseBody, _ := json.Marshal(response)
		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       string(responseBody),
		}, nil
	}

	if interaction.Type != InteractionTypeApplicationCommand {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       "Unsupported interaction type",
		}, nil
	}

	var response DiscordInteractionResponse

	switch interaction.Data.Name {
	case "list":
		response = handleListCommand(ctx, interaction.ChannelID)
	case "add":
		if len(interaction.Data.Options) == 0 {
			response = DiscordInteractionResponse{
				Type: ResponseTypeChannelMessage,
				Data: DiscordInteractionResponseData{
					Content: ShouldInputRssUrl,
					Flags:   MessageFlagEphemeral,
				},
			}
		} else {
			feedURL := interaction.Data.Options[0].Value.(string)
			response = handleAddCommand(ctx, interaction.ChannelID, feedURL)
		}
	case "remove":
		if len(interaction.Data.Options) == 0 {
			response = DiscordInteractionResponse{
				Type: ResponseTypeChannelMessage,
				Data: DiscordInteractionResponseData{
					Content: ShouldInputFeed,
					Flags:   MessageFlagEphemeral,
				},
			}
		} else {
			feedIdentifier := interaction.Data.Options[0].Value.(string)
			response = handleRemoveCommand(ctx, interaction.ChannelID, feedIdentifier)
		}
	case "help":
		response = handleHelpCommand()
	default:
		response = DiscordInteractionResponse{
			Type: ResponseTypeChannelMessage,
			Data: DiscordInteractionResponseData{
				Content: UnknownCommand,
				Flags:   MessageFlagEphemeral,
			},
		}
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Failed to marshal response",
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(responseBody),
	}, nil
}

func main() {
	lambda.Start(handleRequest)
}
