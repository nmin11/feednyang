package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/bwmarrin/discordgo"
	"github.com/mmcdole/gofeed"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Feed struct {
	BlogName       string    `bson:"blogName" json:"blogName"`
	RssURL         string    `bson:"rssUrl" json:"rssUrl"`
	AddedAt        time.Time `bson:"addedAt" json:"addedAt"`
	LastSentTime   time.Time `bson:"lastSentTime" json:"lastSentTime"`
	LastPostTitle  string    `bson:"lastPostTitle" json:"lastPostTitle"`
	TotalPostsSent int       `bson:"totalPostsSent" json:"totalPostsSent"`
}

type DiscordChannel struct {
	ID        string    `bson:"_id" json:"_id"`
	Feeds     []Feed    `bson:"feeds" json:"feeds"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

type LambdaEvent struct {
	Source     string      `json:"source,omitempty"`
	DetailType string      `json:"detail-type,omitempty"`
	Detail     interface{} `json:"detail,omitempty"`
}

type LambdaResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

// 기본 RSS 피드 목록
var techBlogFeeds = []struct {
	Name string
	URL  string
}{
	{"NAVER D2", "https://d2.naver.com/d2.atom"},
	{"토스 테크", "https://toss.tech/rss.xml"},
	{"컬리 기술 블로그", "https://helloworld.kurly.com/feed.xml"},
	{"MUSINSA tech", "https://medium.com/feed/musinsa-tech"},
	{"당근 테크 블로그", "https://medium.com/feed/daangn"},
	{"뱅크샐러드 블로그", "https://blog.banksalad.com/rss.xml"},
	{"요기요 기술블로그", "https://techblog.yogiyo.co.kr/feed"},
	{"Hyperconnect Tech Blog", "https://hyperconnect.github.io/feed.xml"},
	{"LY Corporation Tech Blog", "https://techblog.lycorp.co.jp/ko/feed/index.xml"},
	{"강남언니 블로그", "https://blog.gangnamunni.com/feed.xml"},
	{"데브시스터즈 기술 블로그", "https://tech.devsisters.com/rss.xml"},
	{"SOCAR Tech Blog", "https://tech.socarcorp.kr/feed"},
	{"NHN Cloud Meetup", "https://meetup.nhncloud.com/rss"},
	{"ByteByteGo Newsletter", "https://blog.bytebytego.com/feed"},
	{"Netflix TechBlog", "https://netflixtechblog.com/feed"},
	{"The GitHub Blog", "https://github.blog/engineering/feed"},
	{"Engineering at Slack", "https://slack.engineering/feed"},
	{"The Airbnb Tech Blog", "https://medium.com/feed/airbnb-engineering"},
	{"Spotify Engineering", "https://engineering.atspotify.com/feed"},
	{"Pinterest Engineering", "https://medium.com/feed/@Pinterest_Engineering"},
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

func sendDiscordMessage(channelID string, content string) error {
	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	if botToken == "" {
		return fmt.Errorf("DISCORD_BOT_TOKEN environment variable not set")
	}

	session, err := discordgo.New("Bot " + botToken)
	if err != nil {
		return fmt.Errorf("failed to create Discord session: %v", err)
	}

	_, err = session.ChannelMessageSend(channelID, content)
	if err != nil {
		return fmt.Errorf("failed to send Discord message: %v", err)
	}

	return nil
}

func initializeDefaultChannels(ctx context.Context, client *mongo.Client) error {
	defaultChannelIDs := os.Getenv("DEFAULT_DISCORD_CHANNEL_IDS")
	if defaultChannelIDs == "" {
		log.Println("No default channel IDs provided, skipping initialization")
		return nil
	}

	channelCollection := client.Database("feednyang").Collection("discord_channels")
	channelIDs := strings.SplitSeq(defaultChannelIDs, ",")

	for channelID := range channelIDs {
		channelID = strings.TrimSpace(channelID)
		if channelID == "" {
			continue
		}

		count, err := channelCollection.CountDocuments(ctx, bson.M{"_id": channelID})
		if err != nil {
			log.Printf("Error checking channel %s: %v", channelID, err)
			continue
		}

		if count > 0 {
			continue
		}

		channel := DiscordChannel{
			ID:        channelID,
			Feeds:     []Feed{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		for _, feedInfo := range techBlogFeeds {
			channel.Feeds = append(channel.Feeds, Feed{
				BlogName:       feedInfo.Name,
				RssURL:         feedInfo.URL,
				AddedAt:        time.Now(),
				LastSentTime:   time.Now(),
				LastPostTitle:  "",
				TotalPostsSent: 0,
			})
		}

		_, err = channelCollection.InsertOne(ctx, channel)
		if err != nil {
			log.Printf("Failed to create channel document for %s: %v", channelID, err)
		} else {
			log.Printf("Initialized default channel: %s", channelID)
		}
	}

	return nil
}

func fetchAndProcessFeeds(ctx context.Context, client *mongo.Client) (int, error) {
	err := initializeDefaultChannels(ctx, client)
	if err != nil {
		log.Printf("Failed to initialize default channels: %v", err)
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	fp := gofeed.NewParser()
	fp.Client = httpClient
	fp.UserAgent = "Mozilla/5.0 (compatible; FeedNyang/1.0; +https://github.com/nmin11/feednyang)"
	channelCollection := client.Database("feednyang").Collection("discord_channels")

	totalNewItemsCount := 0

	cursor, err := channelCollection.Find(ctx, bson.M{})
	if err != nil {
		return totalNewItemsCount, fmt.Errorf("failed to find channels: %v", err)
	}
	defer cursor.Close(ctx)

	var channels []DiscordChannel
	if err = cursor.All(ctx, &channels); err != nil {
		return totalNewItemsCount, fmt.Errorf("failed to decode channels: %v", err)
	}

	for _, channel := range channels {
		channelNewItemsCount := 0

		for i, feedConfig := range channel.Feeds {
			var feed *gofeed.Feed
			var err error

			for retry := range 3 {
				feed, err = fp.ParseURLWithContext(feedConfig.RssURL, ctx)
				if err == nil {
					break
				}

				if retry < 2 {
					waitTime := time.Duration((retry+1)*2) * time.Second
					log.Printf("Failed to parse feed %s (attempt %d/3): %v. Retrying in %v", feedConfig.BlogName, retry+1, err, waitTime)
					time.Sleep(waitTime)
				}
			}

			if err != nil {
				log.Printf("Failed to parse feed %s after 3 attempts: %v", feedConfig.BlogName, err)
				continue
			}

			time.Sleep(1 * time.Second)

			for _, item := range feed.Items {
				var publishedAt time.Time
				if item.PublishedParsed != nil {
					publishedAt = *item.PublishedParsed
				} else {
					continue
				}

				if !feedConfig.LastSentTime.IsZero() && publishedAt.Before(feedConfig.LastSentTime) {
					continue
				}

				if feedConfig.LastPostTitle == item.Title {
					continue
				}

				content := fmt.Sprintf(
					"📝 %s\n**🚀 %s**\n🔗 %s",
					feedConfig.BlogName,
					item.Title,
					item.Link,
				)

				err := sendDiscordMessage(channel.ID, content)
				if err != nil {
					log.Printf("Failed to send Discord message for item %s to channel %s: %v", item.Title, channel.ID, err)
					continue
				}

				channel.Feeds[i].LastSentTime = publishedAt
				channel.Feeds[i].LastPostTitle = item.Title
				channel.Feeds[i].TotalPostsSent++

				channelNewItemsCount++
				time.Sleep(1 * time.Second)
			}
		}

		if channelNewItemsCount > 0 {
			channel.UpdatedAt = time.Now()
			_, err = channelCollection.ReplaceOne(ctx, bson.M{"_id": channel.ID}, channel)
			if err != nil {
				log.Printf("Failed to update channel document for %s: %v", channel.ID, err)
			}
		}

		totalNewItemsCount += channelNewItemsCount
		log.Printf("Processed %d new items for channel %s", channelNewItemsCount, channel.ID)
	}

	return totalNewItemsCount, nil
}

func handleRequest(ctx context.Context, event LambdaEvent) (LambdaResponse, error) {
	client, err := connectMongoDB(ctx)
	if err != nil {
		return LambdaResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("Failed to connect to MongoDB: %v", err),
		}, err
	}
	defer client.Disconnect(ctx)

	totalNewItemsCount, err := fetchAndProcessFeeds(ctx, client)
	if err != nil {
		return LambdaResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("Failed to fetch feeds: %v", err),
		}, err
	}

	if totalNewItemsCount == 0 {
		log.Println("No new feed items found across all channels")
		return LambdaResponse{
			StatusCode: 200,
			Body:       "No new feed items found across all channels",
		}, nil
	}

	return LambdaResponse{
		StatusCode: 200,
		Body:       fmt.Sprintf("Successfully processed %d new feed items across all channels", totalNewItemsCount),
	}, nil
}

func main() {
	lambda.Start(handleRequest)
}
