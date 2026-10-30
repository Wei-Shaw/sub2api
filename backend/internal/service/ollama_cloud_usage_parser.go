package service

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var (
	ollamaUsagePercentPattern  = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*%`)
	ollamaUsageWidthPattern    = regexp.MustCompile(`(?i)(?:^|;)\s*width\s*:\s*([0-9]+(?:\.[0-9]+)?)%`)
	ollamaBalancePattern       = regexp.MustCompile(`(?i)(?:balance|credits?)(?:\s+[[:alpha:]]+){0,4}\s*[:\n]?\s*((?:USD\s*)?\$?\s*-?[0-9][0-9,]*(?:\.[0-9]{1,4})?)`)
	ollamaResetPattern         = regexp.MustCompile(`(?i)\breset(?:s|ting)?\s*(?:at|in|on)?\s*[:\-]?\s*([^\n|]+)`)
	ollamaModelFallbackPattern = regexp.MustCompile(`(?i)^(.+?)\s+([0-9][0-9,]*)\s+requests?$`)

	ollamaFiveHourUsageAliases = []string{"session usage", "5 hour usage", "5-hour usage", "5h usage", "5 hour limit", "5-hour limit"}
	ollamaSevenDayUsageAliases = []string{"weekly usage", "7 day usage", "7-day usage", "7d usage", "weekly limit", "7 day limit"}
)

func parseOllamaCloudUsageHTML(body []byte) (*OllamaCloudUsageData, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse settings HTML: %w", err)
	}
	pageText := normalizedNodeText(doc)
	lowerPage := strings.ToLower(pageText)
	if containsAny(lowerPage, "sign in to ollama", "log in to ollama", "continue to sign in") {
		return nil, errOllamaCloudUsageUnauthorizedHTML
	}

	data := &OllamaCloudUsageData{}
	data.Plan = valueBesideLabel(doc, []string{"cloud usage"}, 80)
	if data.Plan == "" {
		data.Plan = valueBesideLabel(doc, []string{"plan", "subscription"}, 80)
	}
	data.FiveHour = parseOllamaUsageWindow(doc, ollamaFiveHourUsageAliases)
	data.SevenDay = parseOllamaUsageWindow(doc, ollamaSevenDayUsageAliases)
	data.Balance = valueBesideLabel(doc, []string{"balance remaining"}, 80)
	if data.Balance == "" {
		if match := ollamaBalancePattern.FindStringSubmatch(pageText); len(match) == 2 {
			data.Balance = strings.Join(strings.Fields(match[1]), "")
		}
	}
	data.Models = parseOllamaModels(doc)

	if data.Plan == "" && data.FiveHour == nil && data.SevenDay == nil && data.Balance == "" && len(data.Models) == 0 {
		return nil, fmt.Errorf("settings HTML does not contain recognizable usage fields")
	}
	return data, nil
}

func parseOllamaUsageWindow(root *html.Node, aliases []string) *OllamaCloudUsageWindow {
	label := findLabelElement(root, aliases)
	if label == nil {
		return nil
	}
	var candidate *OllamaCloudUsageWindow
	block := label
	for depth := 0; block != nil && depth < 6; depth, block = depth+1, block.Parent {
		text := normalizedNodeText(block)
		if len(text) > 600 {
			break
		}
		percent, ok := ollamaUsagePercentFromText(text)
		if !ok {
			percent, ok = ollamaUsagePercentFromTrack(block)
		}
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(text), "remaining") && !strings.Contains(strings.ToLower(text), "used") {
			percent = 100 - percent
		}
		window := &OllamaCloudUsageWindow{UsedPercent: percent}
		window.ResetAt = timeElementValue(block)
		if reset := ollamaResetPattern.FindStringSubmatch(text); len(reset) == 2 {
			window.ResetText = strings.TrimSpace(reset[1])
			if window.ResetAt == nil {
				window.ResetAt = parseOllamaResetTime(window.ResetText)
			}
		}
		if candidate == nil {
			candidate = window
		}
		if window.ResetAt != nil || window.ResetText != "" {
			return window
		}
	}
	return candidate
}

func valueBesideLabel(root *html.Node, aliases []string, maxLen int) string {
	label := findLabelElement(root, aliases)
	if label == nil {
		return ""
	}
	if sibling := nextElementSibling(label); sibling != nil {
		value := strings.Trim(normalizedNodeText(sibling), ":-| ")
		if value != "" && len(value) <= maxLen && !strings.EqualFold(value, "manage") {
			return value
		}
	}
	for depth, block := 0, label.Parent; block != nil && depth < 4; depth, block = depth+1, block.Parent {
		text := normalizedNodeText(block)
		if text == "" || len(text) > maxLen {
			continue
		}
		value := text
		for _, alias := range aliases {
			if index := strings.Index(strings.ToLower(value), alias); index >= 0 {
				value = strings.TrimSpace(value[:index] + " " + value[index+len(alias):])
				break
			}
		}
		value = strings.Trim(value, ":-| ")
		if value != "" && !strings.EqualFold(value, "manage") {
			return value
		}
	}
	return ""
}

func findLabelElement(root *html.Node, aliases []string) *html.Node {
	var best *html.Node
	bestLen := int(^uint(0) >> 1)
	walkHTML(root, func(node *html.Node) {
		if node.Type != html.ElementNode || !isOllamaParserContainer(node.Data) {
			return
		}
		text := strings.ToLower(normalizedNodeText(node))
		if text == "" {
			return
		}
		for _, alias := range aliases {
			if (text == alias || strings.HasPrefix(text, alias+" ") || strings.HasPrefix(text, alias+":")) && len(text) < bestLen {
				best, bestLen = node, len(text)
			}
		}
	})
	return best
}

func parseOllamaModels(root *html.Node) []OllamaCloudUsageModel {
	models := make([]OllamaCloudUsageModel, 0)
	seen := make(map[string]struct{})
	appendModel := func(node *html.Node, modelValue, requestsValue string) {
		model := strings.TrimSpace(modelValue)
		requests, ok := parseOllamaRequestCount(requestsValue)
		window := ollamaModelWindow(node)
		if !ok || model == "" || len(model) > 128 || window == "" {
			return
		}
		key := model + "\x00" + string(window)
		if _, duplicate := seen[key]; duplicate {
			return
		}
		seen[key] = struct{}{}
		models = append(models, OllamaCloudUsageModel{Model: model, Window: window, Requests: requests})
	}

	walkHTML(root, func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
		}
		if _, segment := htmlAttribute(node, "data-usage-segment"); !segment {
			return
		}
		model, modelOK := htmlAttributeInSubtree(node, "data-model")
		requests, requestsOK := htmlAttributeInSubtree(node, "data-requests")
		if modelOK && requestsOK {
			appendModel(node, model, requests)
		}
	})

	// Older settings variants may expose the same narrow attributes without the
	// segment marker. Do not infer request counts from percentages or limits.
	if len(models) == 0 {
		walkHTML(root, func(node *html.Node) {
			if node.Type != html.ElementNode {
				return
			}
			model, modelOK := htmlAttribute(node, "data-model")
			requests, requestsOK := htmlAttribute(node, "data-requests")
			if modelOK && requestsOK {
				appendModel(node, model, requests)
			}
		})
	}

	if len(models) == 0 {
		heading := findLabelElement(root, []string{"models", "available models"})
		for depth, block := 0, parentNode(heading); block != nil && depth < 4; depth, block = depth+1, block.Parent {
			walkHTML(block, func(node *html.Node) {
				if node.Type != html.ElementNode || (node.Data != "li" && node.Data != "code") {
					return
				}
				match := ollamaModelFallbackPattern.FindStringSubmatch(strings.TrimSpace(normalizedNodeText(node)))
				if len(match) == 3 {
					appendModel(node, match[1], match[2])
				}
			})
			if len(models) > 0 {
				break
			}
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Window != models[j].Window {
			return models[i].Window < models[j].Window
		}
		return models[i].Model < models[j].Model
	})
	if len(models) > 100 {
		models = models[:100]
	}
	return models
}

func ollamaModelWindow(node *html.Node) OllamaCloudUsageModelWindow {
	for block := parentNode(node); block != nil; block = block.Parent {
		if value, ok := htmlAttribute(block, "data-usage-window"); ok {
			normalized := strings.ToLower(strings.NewReplacer("-", " ", "_", " ").Replace(value))
			if containsAny(normalized, "five hour", "5 hour", "5h", "session") {
				return OllamaCloudUsageModelWindowFiveHour
			}
			if containsAny(normalized, "seven day", "7 day", "7d", "weekly") {
				return OllamaCloudUsageModelWindowSevenDay
			}
		}
		text := strings.ToLower(normalizedNodeText(block))
		fiveHour := containsAny(text, ollamaFiveHourUsageAliases...)
		sevenDay := containsAny(text, ollamaSevenDayUsageAliases...)
		if fiveHour && !sevenDay {
			return OllamaCloudUsageModelWindowFiveHour
		}
		if sevenDay && !fiveHour {
			return OllamaCloudUsageModelWindowSevenDay
		}
	}
	return ""
}

func parseOllamaRequestCount(value string) (int64, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	requests, err := strconv.ParseInt(value, 10, 64)
	return requests, err == nil && requests >= 0
}

func parentNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}
	return node.Parent
}

func nextElementSibling(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode {
			return sibling
		}
	}
	return nil
}

func timeElementValue(root *html.Node) *time.Time {
	var parsed *time.Time
	walkHTML(root, func(node *html.Node) {
		if parsed != nil || node.Type != html.ElementNode {
			return
		}
		if node.Data == "time" {
			if value, ok := htmlAttribute(node, "datetime"); ok {
				parsed = parseOllamaResetTime(value)
			}
			if parsed != nil {
				return
			}
		}
		if node.Data == "local-time" || htmlClassToken(node, "local-time") {
			if value, ok := htmlAttribute(node, "data-time"); ok {
				parsed = parseOllamaResetTime(value)
			}
		}
	})
	return parsed
}

func ollamaUsagePercentFromText(value string) (float64, bool) {
	match := ollamaUsagePercentPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, false
	}
	percent, err := strconv.ParseFloat(match[1], 64)
	return percent, err == nil && percent >= 0 && percent <= 100
}

func ollamaUsagePercentFromTrack(root *html.Node) (float64, bool) {
	var percent float64
	var found bool
	walkHTML(root, func(node *html.Node) {
		if found || node.Type != html.ElementNode {
			return
		}
		if _, ok := htmlAttribute(node, "data-usage-track"); !ok {
			return
		}
		var segmentTotal float64
		var segmentFound bool
		walkHTML(node, func(segment *html.Node) {
			if segment.Type != html.ElementNode {
				return
			}
			if _, ok := htmlAttribute(segment, "data-usage-segment"); !ok {
				return
			}
			if value, ok := cssWidthPercent(segment); ok {
				segmentTotal += value
				segmentFound = true
			}
		})
		if segmentFound && segmentTotal >= 0 && segmentTotal <= 100 {
			percent, found = segmentTotal, true
			return
		}
		walkHTML(node, func(child *html.Node) {
			if found || child.Type != html.ElementNode {
				return
			}
			if value, ok := cssWidthPercent(child); ok {
				percent, found = value, true
			}
		})
	})
	return percent, found
}

func cssWidthPercent(node *html.Node) (float64, bool) {
	style, ok := htmlAttribute(node, "style")
	if !ok {
		return 0, false
	}
	match := ollamaUsageWidthPattern.FindStringSubmatch(style)
	if len(match) != 2 {
		return 0, false
	}
	percent, err := strconv.ParseFloat(match[1], 64)
	return percent, err == nil && percent >= 0 && percent <= 100
}

func htmlAttribute(node *html.Node, key string) (string, bool) {
	if node == nil {
		return "", false
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val, true
		}
	}
	return "", false
}

func htmlAttributeInSubtree(root *html.Node, key string) (string, bool) {
	var value string
	var found bool
	walkHTML(root, func(node *html.Node) {
		if found || node.Type != html.ElementNode {
			return
		}
		value, found = htmlAttribute(node, key)
	})
	return value, found
}

func htmlClassToken(node *html.Node, token string) bool {
	value, ok := htmlAttribute(node, "class")
	if !ok {
		return false
	}
	for _, className := range strings.Fields(value) {
		if className == token {
			return true
		}
	}
	return false
}

func parseOllamaResetTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "Jan 2, 2006 3:04 PM MST", "January 2, 2006 3:04 PM MST"} {
		if parsed, err := time.Parse(layout, value); err == nil && !parsed.IsZero() {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func normalizedNodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var parts []string
	var collect func(*html.Node)
	collect = func(current *html.Node) {
		if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style" || current.Data == "noscript") {
			return
		}
		if current.Type == html.TextNode {
			if value := strings.TrimSpace(current.Data); value != "" {
				parts = append(parts, value)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(node)
	return strings.Join(strings.Fields(strings.Join(parts, "\n")), " ")
}

func walkHTML(node *html.Node, visit func(*html.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, visit)
	}
}

func isOllamaParserContainer(tag string) bool {
	switch tag {
	case "div", "section", "article", "li", "p", "span", "dt", "dd", "h1", "h2", "h3", "h4":
		return true
	default:
		return false
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
