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
	ollamaBalancePattern       = regexp.MustCompile(`(?i)(?:balance|credits?)(?:\s+[[:alpha:]]+){0,4REDACTED\s*[:\n]?\s*((?:USD\s*)?\$?\s*-?[0-9][0-9,]*(?:\.[0-9]{1,4REDACTED)?)`)
	ollamaResetPattern         = regexp.MustCompile(`(?i)\breset(?:s|ting)?\s*(?:at|in|on)?\s*[:\-]?\s*([^\n|]+)`)
	ollamaModelFallbackPattern = regexp.MustCompile(`(?i)^(.+?)\s+([0-9][0-9,]*)\s+requests?$`)

	ollamaFiveHourUsageAliases = []string{"session usage", "5 hour usage", "5-hour usage", "5h usage", "5 hour limit", "5-hour limit"REDACTED
	ollamaSevenDayUsageAliases = []string{"weekly usage", "7 day usage", "7-day usage", "7d usage", "weekly limit", "7 day limit"REDACTED
)

func parseOllamaCloudUsageHTML(body []byte) (*OllamaCloudUsageData, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse settings HTML: %w", err)
REDACTED
	pageText := normalizedNodeText(doc)
	lowerPage := strings.ToLower(pageText)
	if containsAny(lowerPage, "sign in to ollama", "log in to ollama", "continue to sign in") {
		return nil, errOllamaCloudUsageUnauthorizedHTML
REDACTED

	data := &OllamaCloudUsageData{REDACTED
	data.Plan = valueBesideLabel(doc, []string{"cloud usage"REDACTED, 80)
	if data.Plan == "" {
		data.Plan = valueBesideLabel(doc, []string{"plan", "subscription"REDACTED, 80)
REDACTED
	data.FiveHour = parseOllamaUsageWindow(doc, ollamaFiveHourUsageAliases)
	data.SevenDay = parseOllamaUsageWindow(doc, ollamaSevenDayUsageAliases)
	data.Balance = valueBesideLabel(doc, []string{"balance remaining"REDACTED, 80)
	if data.Balance == "" {
		if match := ollamaBalancePattern.FindStringSubmatch(pageText); len(match) == 2 {
			data.Balance = strings.Join(strings.Fields(match[1]), "")
	REDACTED
REDACTED
	data.Models = parseOllamaModels(doc)

	if data.Plan == "" && data.FiveHour == nil && data.SevenDay == nil && data.Balance == "" && len(data.Models) == 0 {
		return nil, fmt.Errorf("settings HTML does not contain recognizable usage fields")
REDACTED
	return data, nil
REDACTED

func parseOllamaUsageWindow(root *html.Node, aliases []string) *OllamaCloudUsageWindow {
	label := findLabelElement(root, aliases)
	if label == nil {
		return nil
REDACTED
	var candidate *OllamaCloudUsageWindow
	block := label
	for depth := 0; block != nil && depth < 6; depth, block = depth+1, block.Parent {
		text := normalizedNodeText(block)
		if len(text) > 600 {
			break
	REDACTED
		percent, ok := ollamaUsagePercentFromText(text)
		if !ok {
			percent, ok = ollamaUsagePercentFromTrack(block)
	REDACTED
		if !ok {
			continue
	REDACTED
		if strings.Contains(strings.ToLower(text), "remaining") && !strings.Contains(strings.ToLower(text), "used") {
			percent = 100 - percent
	REDACTED
		window := &OllamaCloudUsageWindow{UsedPercent: percentREDACTED
		window.ResetAt = timeElementValue(block)
		if reset := ollamaResetPattern.FindStringSubmatch(text); len(reset) == 2 {
			window.ResetText = strings.TrimSpace(reset[1])
			if window.ResetAt == nil {
				window.ResetAt = parseOllamaResetTime(window.ResetText)
		REDACTED
	REDACTED
		if candidate == nil {
			candidate = window
	REDACTED
		if window.ResetAt != nil || window.ResetText != "" {
			return window
	REDACTED
REDACTED
	return candidate
REDACTED

func valueBesideLabel(root *html.Node, aliases []string, maxLen int) string {
	label := findLabelElement(root, aliases)
	if label == nil {
		return ""
REDACTED
	if sibling := nextElementSibling(label); sibling != nil {
		value := strings.Trim(normalizedNodeText(sibling), ":-| ")
		if value != "" && len(value) <= maxLen && !strings.EqualFold(value, "manage") {
			return value
	REDACTED
REDACTED
	for depth, block := 0, label.Parent; block != nil && depth < 4; depth, block = depth+1, block.Parent {
		text := normalizedNodeText(block)
		if text == "" || len(text) > maxLen {
			continue
	REDACTED
		value := text
		for _, alias := range aliases {
			if index := strings.Index(strings.ToLower(value), alias); index >= 0 {
				value = strings.TrimSpace(value[:index] + " " + value[index+len(alias):])
				break
		REDACTED
	REDACTED
		value = strings.Trim(value, ":-| ")
		if value != "" && !strings.EqualFold(value, "manage") {
			return value
	REDACTED
REDACTED
	return ""
REDACTED

func findLabelElement(root *html.Node, aliases []string) *html.Node {
	var best *html.Node
	bestLen := int(^uint(0) >> 1)
	walkHTML(root, func(node *html.Node) {
		if node.Type != html.ElementNode || !isOllamaParserContainer(node.Data) {
			return
	REDACTED
		text := strings.ToLower(normalizedNodeText(node))
		if text == "" {
			return
	REDACTED
		for _, alias := range aliases {
			if (text == alias || strings.HasPrefix(text, alias+" ") || strings.HasPrefix(text, alias+":")) && len(text) < bestLen {
				best, bestLen = node, len(text)
		REDACTED
	REDACTED
REDACTED)
	return best
REDACTED

func parseOllamaModels(root *html.Node) []OllamaCloudUsageModel {
	models := make([]OllamaCloudUsageModel, 0)
	seen := make(map[string]struct{REDACTED)
	appendModel := func(node *html.Node, modelValue, requestsValue string) {
		model := strings.TrimSpace(modelValue)
		requests, ok := parseOllamaRequestCount(requestsValue)
		window := ollamaModelWindow(node)
		if !ok || model == "" || len(model) > 128 || window == "" {
			return
	REDACTED
		key := model + "\x00" + string(window)
		if _, duplicate := seen[key]; duplicate {
			return
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		models = append(models, OllamaCloudUsageModel{Model: model, Window: window, Requests: requestsREDACTED)
REDACTED

	walkHTML(root, func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
	REDACTED
		if _, segment := htmlAttribute(node, "data-usage-segment"); !segment {
			return
	REDACTED
		model, modelOK := htmlAttributeInSubtree(node, "data-model")
		requests, requestsOK := htmlAttributeInSubtree(node, "data-requests")
		if modelOK && requestsOK {
			appendModel(node, model, requests)
	REDACTED
REDACTED)

	// Older settings variants may expose the same narrow attributes without the
	// segment marker. Do not infer request counts from percentages or limits.
	if len(models) == 0 {
		walkHTML(root, func(node *html.Node) {
			if node.Type != html.ElementNode {
				return
		REDACTED
			model, modelOK := htmlAttribute(node, "data-model")
			requests, requestsOK := htmlAttribute(node, "data-requests")
			if modelOK && requestsOK {
				appendModel(node, model, requests)
		REDACTED
	REDACTED)
REDACTED

	if len(models) == 0 {
		heading := findLabelElement(root, []string{"models", "available models"REDACTED)
		for depth, block := 0, parentNode(heading); block != nil && depth < 4; depth, block = depth+1, block.Parent {
			walkHTML(block, func(node *html.Node) {
				if node.Type != html.ElementNode || (node.Data != "li" && node.Data != "code") {
					return
			REDACTED
				match := ollamaModelFallbackPattern.FindStringSubmatch(strings.TrimSpace(normalizedNodeText(node)))
				if len(match) == 3 {
					appendModel(node, match[1], match[2])
			REDACTED
		REDACTED)
			if len(models) > 0 {
				break
		REDACTED
	REDACTED
REDACTED

	sort.Slice(models, func(i, j int) bool {
		if models[i].Window != models[j].Window {
			return models[i].Window < models[j].Window
	REDACTED
		return models[i].Model < models[j].Model
REDACTED)
	if len(models) > 100 {
		models = models[:100]
REDACTED
	return models
REDACTED

func ollamaModelWindow(node *html.Node) OllamaCloudUsageModelWindow {
	for block := parentNode(node); block != nil; block = block.Parent {
		if value, ok := htmlAttribute(block, "data-usage-window"); ok {
			normalized := strings.ToLower(strings.NewReplacer("-", " ", "_", " ").Replace(value))
			if containsAny(normalized, "five hour", "5 hour", "5h", "session") {
				return OllamaCloudUsageModelWindowFiveHour
		REDACTED
			if containsAny(normalized, "seven day", "7 day", "7d", "weekly") {
				return OllamaCloudUsageModelWindowSevenDay
		REDACTED
	REDACTED
		text := strings.ToLower(normalizedNodeText(block))
		fiveHour := containsAny(text, ollamaFiveHourUsageAliases...)
		sevenDay := containsAny(text, ollamaSevenDayUsageAliases...)
		if fiveHour && !sevenDay {
			return OllamaCloudUsageModelWindowFiveHour
	REDACTED
		if sevenDay && !fiveHour {
			return OllamaCloudUsageModelWindowSevenDay
	REDACTED
REDACTED
	return ""
REDACTED

func parseOllamaRequestCount(value string) (int64, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	requests, err := strconv.ParseInt(value, 10, 64)
	return requests, err == nil && requests >= 0
REDACTED

func parentNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
REDACTED
	return node.Parent
REDACTED

func nextElementSibling(node *html.Node) *html.Node {
	if node == nil {
		return nil
REDACTED
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode {
			return sibling
	REDACTED
REDACTED
	return nil
REDACTED

func timeElementValue(root *html.Node) *time.Time {
	var parsed *time.Time
	walkHTML(root, func(node *html.Node) {
		if parsed != nil || node.Type != html.ElementNode {
			return
	REDACTED
		if node.Data == "time" {
			if value, ok := htmlAttribute(node, "datetime"); ok {
				parsed = parseOllamaResetTime(value)
		REDACTED
			if parsed != nil {
				return
		REDACTED
	REDACTED
		if node.Data == "local-time" || htmlClassToken(node, "local-time") {
			if value, ok := htmlAttribute(node, "data-time"); ok {
				parsed = parseOllamaResetTime(value)
		REDACTED
	REDACTED
REDACTED)
	return parsed
REDACTED

func ollamaUsagePercentFromText(value string) (float64, bool) {
	match := ollamaUsagePercentPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, false
REDACTED
	percent, err := strconv.ParseFloat(match[1], 64)
	return percent, err == nil && percent >= 0 && percent <= 100
REDACTED

func ollamaUsagePercentFromTrack(root *html.Node) (float64, bool) {
	var percent float64
	var found bool
	walkHTML(root, func(node *html.Node) {
		if found || node.Type != html.ElementNode {
			return
	REDACTED
		if _, ok := htmlAttribute(node, "data-usage-track"); !ok {
			return
	REDACTED
		var segmentTotal float64
		var segmentFound bool
		walkHTML(node, func(segment *html.Node) {
			if segment.Type != html.ElementNode {
				return
		REDACTED
			if _, ok := htmlAttribute(segment, "data-usage-segment"); !ok {
				return
		REDACTED
			if value, ok := cssWidthPercent(segment); ok {
				segmentTotal += value
				segmentFound = true
		REDACTED
	REDACTED)
		if segmentFound && segmentTotal >= 0 && segmentTotal <= 100 {
			percent, found = segmentTotal, true
			return
	REDACTED
		walkHTML(node, func(child *html.Node) {
			if found || child.Type != html.ElementNode {
				return
		REDACTED
			if value, ok := cssWidthPercent(child); ok {
				percent, found = value, true
		REDACTED
	REDACTED)
REDACTED)
	return percent, found
REDACTED

func cssWidthPercent(node *html.Node) (float64, bool) {
	style, ok := htmlAttribute(node, "style")
	if !ok {
		return 0, false
REDACTED
	match := ollamaUsageWidthPattern.FindStringSubmatch(style)
	if len(match) != 2 {
		return 0, false
REDACTED
	percent, err := strconv.ParseFloat(match[1], 64)
	return percent, err == nil && percent >= 0 && percent <= 100
REDACTED

func htmlAttribute(node *html.Node, key string) (string, bool) {
	if node == nil {
		return "", false
REDACTED
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val, true
	REDACTED
REDACTED
	return "", false
REDACTED

func htmlAttributeInSubtree(root *html.Node, key string) (string, bool) {
	var value string
	var found bool
	walkHTML(root, func(node *html.Node) {
		if found || node.Type != html.ElementNode {
			return
	REDACTED
		value, found = htmlAttribute(node, key)
REDACTED)
	return value, found
REDACTED

func htmlClassToken(node *html.Node, token string) bool {
	value, ok := htmlAttribute(node, "class")
	if !ok {
		return false
REDACTED
	for _, className := range strings.Fields(value) {
		if className == token {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func parseOllamaResetTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "Jan 2, 2006 3:04 PM MST", "January 2, 2006 3:04 PM MST"REDACTED {
		if parsed, err := time.Parse(layout, value); err == nil && !parsed.IsZero() {
			parsed = parsed.UTC()
			return &parsed
	REDACTED
REDACTED
	return nil
REDACTED

func normalizedNodeText(node *html.Node) string {
	if node == nil {
		return ""
REDACTED
	var parts []string
	var collect func(*html.Node)
	collect = func(current *html.Node) {
		if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style" || current.Data == "noscript") {
			return
	REDACTED
		if current.Type == html.TextNode {
			if value := strings.TrimSpace(current.Data); value != "" {
				parts = append(parts, value)
		REDACTED
	REDACTED
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
	REDACTED
REDACTED
	collect(node)
	return strings.Join(strings.Fields(strings.Join(parts, "\n")), " ")
REDACTED

func walkHTML(node *html.Node, visit func(*html.Node)) {
	if node == nil {
		return
REDACTED
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, visit)
REDACTED
REDACTED

func isOllamaParserContainer(tag string) bool {
	switch tag {
	case "div", "section", "article", "li", "p", "span", "dt", "dd", "h1", "h2", "h3", "h4":
		return true
	default:
		return false
REDACTED
REDACTED

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
	REDACTED
REDACTED
	return false
REDACTED
