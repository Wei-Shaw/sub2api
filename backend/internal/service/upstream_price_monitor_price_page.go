package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"golang.org/x/net/html"
)

const x5m5xPricingPageURL = "https://api.x5m5x.com/pricing/"
const x5m5xPricingPageMaxBytes = 2 << 20

type UpstreamPricePageFetcher interface {
	FetchPerRequestPrices(context.Context) (map[string]domain.UpstreamPriceVector, error)
}

type x5m5xPricePageFetcher struct{ client *http.Client }

func NewX5M5XPricePageFetcher() UpstreamPricePageFetcher {
	return &x5m5xPricePageFetcher{client: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !strings.EqualFold(req.URL.Hostname(), "api.x5m5x.com") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}}
}

func (f *x5m5xPricePageFetcher) FetchPerRequestPrices(ctx context.Context) (map[string]domain.UpstreamPriceVector, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, x5m5xPricingPageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch x5m5x pricing page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch x5m5x pricing page: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, x5m5xPricingPageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read x5m5x pricing page: %w", err)
	}
	if len(body) > x5m5xPricingPageMaxBytes {
		return nil, errors.New("x5m5x pricing page is too large")
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse x5m5x pricing page: %w", err)
	}
	out := make(map[string]domain.UpstreamPriceVector)
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type == html.ElementNode && node.Data == "tr" && hasHTMLClass(node, "request-table-row") {
			name := strings.TrimSpace(firstDescendantText(node, "th", "m-name"))
			prices := descendantPriceTexts(node, "request-cost")
			if name == "" || len(prices) != 3 {
				return nil
			}
			key := strings.ToLower(name)
			if _, duplicate := out[key]; duplicate {
				return fmt.Errorf("duplicate x5m5x per-request model %q", name)
			}
			small, middle, large := prices[0], prices[1], prices[2]
			out[key] = domain.UpstreamPriceVector{
				PerRequestLTE256K: &small, PerRequest256K512K: &middle, PerRequestGT512K: &large,
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("x5m5x pricing page contained no per-request models")
	}
	return out, nil
}

func hasHTMLClass(node *html.Node, className string) bool {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			for _, value := range strings.Fields(attr.Val) {
				if value == className {
					return true
				}
			}
		}
	}
	return false
}

func firstDescendantText(node *html.Node, tag, className string) string {
	if node.Type == html.ElementNode && node.Data == tag && hasHTMLClass(node, className) {
		return htmlNodeText(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if value := firstDescendantText(child, tag, className); value != "" {
			return value
		}
	}
	return ""
}

func descendantPriceTexts(node *html.Node, className string) []float64 {
	var values []float64
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && hasHTMLClass(current, className) {
			raw := strings.NewReplacer("¥", "", "￥", "", ",", "").Replace(strings.TrimSpace(htmlNodeText(current)))
			if value, err := strconv.ParseFloat(raw, 64); err == nil && value >= 0 {
				values = append(values, value)
			}
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return values
}

func htmlNodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.TrimSpace(builder.String())
}
