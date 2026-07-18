package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/html"
)

// SearchResult represents a web search result
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchClient performs web searches
type SearchClient struct {
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewSearchClient creates a new search client
func NewSearchClient(logger *logrus.Logger) *SearchClient {
	return &SearchClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		logger: logger,
	}
}

// Search performs a web search and returns results.
// Uses DuckDuckGo HTML search (no API key needed) as the default search engine.
func (c *SearchClient) Search(ctx context.Context, query string, maxResults int) ([]*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create search request")
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoBackend/1.0)")

	c.logger.WithField("query", query).Debug("Performing web search")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("Failed to execute search request")
		return nil, fmt.Errorf("failed to execute search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.WithField("status_code", resp.StatusCode).Error("Search request returned non-OK status")
		return nil, fmt.Errorf("search request returned status %d", resp.StatusCode)
	}

	results, err := parseSearchResults(resp.Body, maxResults)
	if err != nil {
		c.logger.WithError(err).Error("Failed to parse search results")
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"query":        query,
		"result_count": len(results),
	}).Debug("Search completed")

	return results, nil
}

// parseSearchResults parses HTML search results from DuckDuckGo.
func parseSearchResults(r io.Reader, maxResults int) ([]*SearchResult, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var results []*SearchResult
	var currentResult *SearchResult
	var inResult bool
	var inLink bool
	var inSnippet bool

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= maxResults {
			return
		}

		if n.Type == html.ElementNode {
			// Detect result container
			if n.Data == "div" && hasClass(n, "result") {
				inResult = true
				currentResult = &SearchResult{}
			}

			// Detect result link (title)
			if inResult && n.Data == "a" && hasClass(n, "result__a") {
				inLink = true
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						currentResult.URL = cleanURL(attr.Val)
					}
				}
			}

			// Detect result snippet
			if inResult && n.Data == "a" && hasClass(n, "result__snippet") {
				inSnippet = true
			}
		}

		if n.Type == html.TextNode {
			if inLink {
				currentResult.Title += n.Data
			}
			if inSnippet {
				currentResult.Snippet += n.Data
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}

		if n.Type == html.ElementNode {
			if n.Data == "a" && inLink && hasClass(n, "result__a") {
				inLink = false
			}
			if n.Data == "a" && inSnippet && hasClass(n, "result__snippet") {
				inSnippet = false
			}
			if n.Data == "div" && inResult && hasClass(n, "result") {
				inResult = false
				if currentResult != nil && currentResult.URL != "" {
					currentResult.Title = strings.TrimSpace(currentResult.Title)
					currentResult.Snippet = strings.TrimSpace(currentResult.Snippet)
					results = append(results, currentResult)
				}
				currentResult = nil
			}
		}
	}

	walk(doc)

	return results, nil
}

// hasClass checks if an HTML node has the specified class attribute.
func hasClass(n *html.Node, class string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			classes := strings.Fields(attr.Val)
			for _, c := range classes {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}

// cleanURL decodes and cleans a URL from DuckDuckGo's redirect format.
func cleanURL(rawURL string) string {
	// DuckDuckGo uses a redirect wrapper like //duckduckgo.com/l/?uddg=...
	const prefix = "uddg="
	if idx := strings.Index(rawURL, prefix); idx != -1 {
		rawURL = rawURL[idx+len(prefix):]
		if ampIdx := strings.Index(rawURL, "&"); ampIdx != -1 {
			rawURL = rawURL[:ampIdx]
		}
		decoded, err := url.QueryUnescape(rawURL)
		if err == nil {
			rawURL = decoded
		}
	}

	// Remove DuckDuckGo l.php redirect prefix
	rawURL = strings.TrimPrefix(rawURL, "//duckduckgo.com/l/?")
	rawURL = strings.TrimPrefix(rawURL, "/l/?")

	return rawURL
}