package worker

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRSSFeedParsing(t *testing.T) {
	tests := []struct {
		name          string
		xmlContent    string
		expectError   bool
		expectedTitle string
		expectedLink  string
		expectedDesc  string
		expectedItems int
	}{
		{
			name: "Valid XML",
			xmlContent: `
				<?xml version="1.0" encoding="UTF-8" ?>
				<rss version="2.0">
				  <channel>
					<title>Test Feed</title>
					<link>http://example.com</link>
					<description>This is a test feed</description>
					<item>
					  <title>Item 1</title>
					  <link>http://example.com/item1</link>
					  <description>Description for item 1</description>
					  <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
					</item>
					<item>
					  <title>Item 2</title>
					  <link>http://example.com/item2</link>
					  <description>Description for item 2</description>
					  <pubDate>Tue, 03 Jan 2006 15:04:05 -0700</pubDate>
					</item>
				  </channel>
				</rss>`,
			expectError:   false,
			expectedTitle: "Test Feed",
			expectedLink:  "http://example.com",
			expectedDesc:  "This is a test feed",
			expectedItems: 2,
		},
		{
			name: "Invalid XML",
			xmlContent: `
				<?xml version="1.0" encoding="UTF-8" ?>
				<rss version="2.0">
				  <channel>
					<title>Test Feed
				  </channel>
				</rss>`,
			expectError: true,
		},
		{
			name: "Empty Items",
			xmlContent: `
				<?xml version="1.0" encoding="UTF-8" ?>
				<rss version="2.0">
				  <channel>
					<title>Empty Feed</title>
					<link>http://empty.com</link>
				  </channel>
				</rss>`,
			expectError:   false,
			expectedTitle: "Empty Feed",
			expectedLink:  "http://empty.com",
			expectedDesc:  "",
			expectedItems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var feed RSSFeed
			err := xml.Unmarshal([]byte(tt.xmlContent), &feed)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedTitle, feed.Channel.Title)
				assert.Equal(t, tt.expectedLink, feed.Channel.Link)
				assert.Equal(t, tt.expectedDesc, feed.Channel.Description)
				assert.Len(t, feed.Channel.Items, tt.expectedItems)
			}
		})
	}
}
