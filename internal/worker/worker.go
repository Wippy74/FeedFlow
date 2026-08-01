package worker

import (
	"NewsAggregator/internal/database/storage"
	"NewsAggregator/internal/model"
	"context"
	"database/sql"
	"encoding/xml"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

func Start(db *storage.Repository, fetchInterval time.Duration, concurrency int) error {
	ticker := time.NewTicker(fetchInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		feeds, err := db.GetNextFeedsToFetch(ctx, concurrency)
		cancel()
		if err != nil {
			log.Println(err)
			continue
		}
		if len(feeds) == 0 {
			continue
		}

		wg := sync.WaitGroup{}
		for _, feed := range feeds {
			wg.Add(1)
			go func(f model.Feed) {
				defer wg.Done()
				scrapeFeed(db, f)
			}(feed)
		}
		wg.Wait()
	}
}

func scrapeFeed(db *storage.Repository, feed model.Feed) {
	ctx := context.Background()
	_ = db.MarkFeedFetched(ctx, feed.ID)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(feed.Url)
	if err != nil {
		log.Printf("Error during feed downloading %s: %v", feed.Name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		log.Printf("Feed %s returned %d", feed.Name, resp.StatusCode)
		return
	}

	var rss RSSFeed
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&rss); err != nil {
		log.Printf("Error while parsing XML for feed %s: %v", feed.Name, err)
		return
	}

	postSaved := 0
	for _, item := range rss.Channel.Items {
		pubDate, err := time.Parse(time.RFC1123Z, item.PubDate)
		if err != nil {
			pubDate, err = time.Parse(time.RFC1123, item.PubDate)
			if err != nil {
				pubDate = time.Now()
			}
		}
		description := sql.NullString{
			String: item.Description,
			Valid:  item.Description != "",
		}

		err = db.SavePost(ctx, model.Post{
			ID:          uuid.New(),
			Title:       item.Title,
			Description: description,
			PublishedAt: pubDate,
			Url:         item.Link,
			FeedID:      feed.ID,
		})

		if err != nil {
			log.Printf("Could not save post %s: %v", item.Title, err)
			continue
		}
		postSaved++
	}
	log.Printf("Feed %s succsessfully updated. Saving new posts: %d", feed.Name, postSaved)
}
