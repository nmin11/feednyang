package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
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
	LastPostLink   string    `bson:"lastPostLink" json:"lastPostLink"`
	TotalPostsSent int       `bson:"totalPostsSent" json:"totalPostsSent"`
}

type DiscordChannel struct {
	ID        string    `bson:"_id" json:"_id"`
	Feeds     []Feed    `bson:"feeds" json:"feeds"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

type LambdaEvent struct {
	Source     string `json:"source,omitempty"`
	DetailType string `json:"detail-type,omitempty"`
	Detail     any    `json:"detail,omitempty"`
}

type LambdaResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

type channelProcessResult struct {
	channel     DiscordChannel
	newItems    int
	needsUpdate bool
	err         error
}

type feedParseResult struct {
	feed Feed
	err  error
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
	{"The GitHub Blog", "https://github.blog/feed"},
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

func ensureDefaultChannels(ctx context.Context, channelCollection *mongo.Collection, fp *gofeed.Parser) error {
	defaultChannelIDs := os.Getenv("DEFAULT_DISCORD_CHANNEL_IDS")
	if defaultChannelIDs == "" {
		log.Println("No default channel IDs provided, skipping initialization")
		return nil
	}

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

		var feedWg sync.WaitGroup
		feedResults := make(chan feedParseResult, len(techBlogFeeds))

		for _, feedInfo := range techBlogFeeds {
			feedWg.Add(1)
			go func(info struct{ Name, URL string }) {
				defer feedWg.Done()

				now := time.Now()
				var lastPostLink string
				var lastSentTime time.Time = now

				feed, err := fp.ParseURL(info.URL)
				if err != nil {
					log.Printf("Failed to parse feed %s during initialization: %v", info.Name, err)
				} else if len(feed.Items) > 0 {
					lastPostLink = feed.Items[0].Link
					if feed.Items[0].PublishedParsed != nil {
						lastSentTime = *feed.Items[0].PublishedParsed
					}
				}

				feedResult := feedParseResult{
					feed: Feed{
						BlogName:       info.Name,
						RssURL:         info.URL,
						AddedAt:        now,
						LastSentTime:   lastSentTime,
						LastPostLink:   lastPostLink,
						TotalPostsSent: 0,
					},
					err: err,
				}

				feedResults <- feedResult
				time.Sleep(100 * time.Millisecond)
			}(feedInfo)
		}

		go func() {
			feedWg.Wait()
			close(feedResults)
		}()

		for result := range feedResults {
			channel.Feeds = append(channel.Feeds, result.feed)
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

func processChannelFeeds(ctx context.Context, channel DiscordChannel, fp *gofeed.Parser) channelProcessResult {
	channelNewItemsCount := 0
	needsUpdate := false

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

		time.Sleep(250 * time.Millisecond)

		firstItem := true
		for _, item := range feed.Items {
			if feedConfig.LastPostLink == item.Link {
				break
			}

			if item.PublishedParsed != nil && item.PublishedParsed.Before(feedConfig.LastSentTime) {
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

			if firstItem {
				channel.Feeds[i].LastPostLink = item.Link
				firstItem = false
			}
			channel.Feeds[i].LastSentTime = time.Now()
			channel.Feeds[i].TotalPostsSent++

			channelNewItemsCount++
			needsUpdate = true
			time.Sleep(500 * time.Millisecond)
		}
	}

	return channelProcessResult{
		channel:     channel,
		newItems:    channelNewItemsCount,
		needsUpdate: needsUpdate,
		err:         nil,
	}
}

func fetchAndProcessFeeds(ctx context.Context, client *mongo.Client) (int, error) {
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

	err := ensureDefaultChannels(ctx, channelCollection, fp)
	if err != nil {
		log.Printf("Failed to ensure default channels: %v", err)
	}

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

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 3)
	results := make(chan channelProcessResult, len(channels))

	for _, channel := range channels {
		wg.Add(1)
		go func(ch DiscordChannel) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := processChannelFeeds(ctx, ch, fp)
			results <- result
		}(channel)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			log.Printf("Error processing channel %s: %v", result.channel.ID, result.err)
			continue
		}

		if result.needsUpdate {
			result.channel.UpdatedAt = time.Now()
			_, err = channelCollection.ReplaceOne(ctx, bson.M{"_id": result.channel.ID}, result.channel)
			if err != nil {
				log.Printf("Failed to update channel document for %s: %v", result.channel.ID, err)
			}
		}

		totalNewItemsCount += result.newItems
		log.Printf("Processed %d new items for channel %s", result.newItems, result.channel.ID)
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
