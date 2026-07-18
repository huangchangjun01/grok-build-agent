package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/html"
)

const defaultMaxFetchSize = 100 * 1024 // 100KB

// FetchResult represents a fetched web page
type FetchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	StatusCode  int    `json:"status_code"`
	Size        int    `json:"size"`
}

// FetchClient fetches web page content
type FetchClient struct {
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewFetchClient creates a new fetch client
func NewFetchClient(logger *logrus.Logger) *FetchClient {
	return &FetchClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// Fetch fetches the content of a URL.
// Returns the page content as plain text (strips HTML tags).
func (c *FetchClient) Fetch(ctx context.Context, url string, maxSize int) (*FetchResult, error) {
	if maxSize <= 0 {
		maxSize = defaultMaxFetchSize
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.logger.WithError(err).WithField("url", url).Error("Failed to create fetch request")
		return nil, fmt.Errorf("failed to create fetch request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoBackend/1.0)")

	c.logger.WithField("url", url).Debug("Fetching web page")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).WithField("url", url).Error("Failed to execute fetch request")
		return nil, fmt.Errorf("failed to execute fetch request: %w", err)
	}
	defer resp.Body.Close()

	// Limit the amount of data read
	limitedReader := io.LimitReader(resp.Body, int64(maxSize))

	doc, err := html.Parse(limitedReader)
	if err != nil {
		c.logger.WithError(err).WithField("url", url).Error("Failed to parse HTML")
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	title, bodyText := extractText(doc)

	content := strings.TrimSpace(bodyText)

	result := &FetchResult{
		URL:         url,
		Title:       strings.TrimSpace(title),
		Content:     content,
		ContentType: contentType,
		StatusCode:  resp.StatusCode,
		Size:        len(content),
	}

	c.logger.WithFields(logrus.Fields{
		"url":          url,
		"status_code":  resp.StatusCode,
		"content_size": len(content),
	}).Debug("Fetch completed")

	return result, nil
}

// extractText walks the HTML tree and extracts text content.
// It skips script, style, noscript, and other non-content elements.
// Returns the page title and body text separately.
func extractText(doc *html.Node) (title string, bodyText string) {
	var sb strings.Builder
	var inTitle bool
	var inSkip bool
	skipDepth := 0

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				inTitle = true
			case "script", "style", "noscript", "svg", "head", "meta", "link":
				if !inSkip {
					inSkip = true
					skipDepth = 0
				}
			}

			if inSkip {
				skipDepth++
			}
		}

		if n.Type == html.TextNode && !inSkip {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if inTitle {
					title += text + " "
				} else {
					sb.WriteString(text)
					sb.WriteString(" ")
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}

		if n.Type == html.ElementNode {
			if n.Data == "title" {
				inTitle = false
			}

			if inSkip {
				skipDepth--
				if skipDepth == 0 {
					inSkip = false
				}
			}
		}
	}

	walk(doc)

	return strings.TrimSpace(title), strings.TrimSpace(sb.String())
}