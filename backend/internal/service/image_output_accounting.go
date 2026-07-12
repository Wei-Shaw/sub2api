package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIImageOutputCounter struct {
	seen         map[string]struct{}
	seenSizes    map[string]string
	seenBase64   map[string]string
	seenURLs     map[string]string
	seenOrder    []string
	dataSizes    []string
	count        int
	maxDataCount int
}

func newOpenAIImageOutputCounter() *openAIImageOutputCounter {
	return &openAIImageOutputCounter{
		seen:       make(map[string]struct{}),
		seenSizes:  make(map[string]string),
		seenBase64: make(map[string]string),
		seenURLs:   make(map[string]string),
	}
}

func (c *openAIImageOutputCounter) Count() int {
	if c == nil {
		return 0
	}
	if c.maxDataCount > c.count {
		return c.maxDataCount
	}
	return c.count
}

func (c *openAIImageOutputCounter) Sizes() []string {
	if c == nil {
		return nil
	}
	sizes := make([]string, 0, len(c.seenOrder)+len(c.dataSizes))
	for _, key := range c.seenOrder {
		if size := strings.TrimSpace(c.seenSizes[key]); size != "" {
			sizes = append(sizes, size)
		}
	}
	if len(sizes) == 0 && len(c.dataSizes) > 0 {
		sizes = append(sizes, c.dataSizes...)
	}
	if len(sizes) == 0 {
		return nil
	}
	return sizes
}

// SizesPerSlot 与 seenOrder 严格对齐：每个 slot 一个 size，size 缺失/auto 时占位空串。
// 与 Base64Payloads() 同序、同长度，供 §5 回包图片分辨率自检消费。
// 当 seenOrder 为空但存在 dataSizes 时，回退到 dataSizes（保持与 Sizes() 同源）。
func (c *openAIImageOutputCounter) SizesPerSlot() []string {
	if c == nil {
		return nil
	}
	if len(c.seenOrder) == 0 {
		if len(c.dataSizes) == 0 {
			return nil
		}
		out := make([]string, len(c.dataSizes))
		copy(out, c.dataSizes)
		return out
	}
	out := make([]string, len(c.seenOrder))
	for i, key := range c.seenOrder {
		out[i] = strings.TrimSpace(c.seenSizes[key])
	}
	return out
}

// Base64Payloads 与 seenOrder 严格对齐：每个 slot 一个 b64 内容，URL 模式或未知占位空串。
// 与 SizesPerSlot() 同序、同长度。仅当对应 slot 缺失/auto size 时调用方才需要消费此 payload。
// 当 seenOrder 为空（仅命中 dataSizes 路径）时返回 nil——dataSizes 路径不携带 b64 内容。
func (c *openAIImageOutputCounter) Base64Payloads() []string {
	if c == nil || len(c.seenOrder) == 0 {
		return nil
	}
	out := make([]string, len(c.seenOrder))
	for i, key := range c.seenOrder {
		out[i] = c.seenBase64[key]
	}
	return out
}

func (c *openAIImageOutputCounter) URLs() []string {
	if c == nil || len(c.seenOrder) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.seenOrder))
	for _, key := range c.seenOrder {
		if u := strings.TrimSpace(c.seenURLs[key]); u != "" {
			out = append(out, u)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *openAIImageOutputCounter) AddJSONResponse(body []byte) {
	if c == nil || len(body) == 0 || !gjson.ValidBytes(body) {
		return
	}
	c.addDataArray(gjson.GetBytes(body, "data"))
	c.addOutputArray(gjson.GetBytes(body, "output"))
	c.addOutputArray(gjson.GetBytes(body, "response.output"))
}

func (c *openAIImageOutputCounter) AddSSEData(data []byte) {
	if c == nil || len(data) == 0 || strings.TrimSpace(string(data)) == "[DONE]" || !gjson.ValidBytes(data) {
		return
	}
	root := gjson.ParseBytes(data)
	c.addDataArray(root.Get("data"))
	eventType := strings.TrimSpace(root.Get("type").String())
	switch eventType {
	case "response.output_item.done":
		c.addImageOutputItem(root.Get("item"))
	case "response.completed", "response.done":
		c.addOutputArray(root.Get("response.output"))
	case "image_generation.completed":
		if item := root.Get("item"); item.Exists() {
			c.addImageOutputItem(item)
			return
		}
		if output := root.Get("output"); output.Exists() {
			c.addImageOutputItem(output)
			return
		}
		c.addImageOutputItem(root)
	}
}

func (c *openAIImageOutputCounter) AddSSEBody(body string) {
	if c == nil || strings.TrimSpace(body) == "" {
		return
	}
	forEachOpenAISSEDataPayload(body, c.AddSSEData)
}

func (c *openAIImageOutputCounter) addDataArray(data gjson.Result) {
	if !data.IsArray() {
		return
	}
	items := data.Array()
	imageCount := 0
	sizes := make([]string, 0, len(items))
	for _, item := range items {
		if !item.IsObject() {
			continue
		}
		hasImageOutput := strings.TrimSpace(item.Get("url").String()) != "" ||
			strings.TrimSpace(item.Get("b64_json").String()) != ""
		if !hasImageOutput {
			continue
		}
		imageCount++
		if size := strings.TrimSpace(item.Get("size").String()); size != "" {
			sizes = append(sizes, size)
		}
		// 修复：对 data 数组中的每个项目也调用 addImageOutputItem 来处理 b64_json 字段
		c.addImageOutputItem(item)
	}
	if imageCount > c.maxDataCount {
		c.maxDataCount = imageCount
	}
	if len(sizes) > 0 {
		c.dataSizes = sizes
	}
}

func (c *openAIImageOutputCounter) addOutputArray(output gjson.Result) {
	if !output.IsArray() {
		return
	}
	output.ForEach(func(_, item gjson.Result) bool {
		c.addImageOutputItem(item)
		return true
	})
}

func (c *openAIImageOutputCounter) addImageOutputItem(item gjson.Result) {
	if !item.Exists() || !item.IsObject() {
		return
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	if itemType != "" && itemType != "image_generation_call" && itemType != "image_generation.completed" {
		return
	}
	if strings.Contains(strings.ToLower(item.Raw), "partial_image") {
		return
	}
	// 分别取 b64 与 url：b64 优先（用于 §5 解码），url 仅用于 hash key。
	b64Payload := strings.TrimSpace(item.Get("b64_json").String())
	if b64Payload == "" {
		b64Payload = strings.TrimSpace(item.Get("result").String())
	}
	urlPayload := strings.TrimSpace(item.Get("url").String())

	result := b64Payload
	if result == "" {
		result = urlPayload
	}
	if result == "" {
		return
	}
	key := strings.TrimSpace(item.Get("id").String())
	if key == "" {
		key = strings.TrimSpace(item.Get("call_id").String())
	}
	if key == "" {
		key = hashOpenAIImageOutputResult(result)
	}
	if key == "" {
		return
	}
	size := strings.TrimSpace(item.Get("size").String())
	if _, exists := c.seen[key]; exists {
		if size != "" && strings.TrimSpace(c.seenSizes[key]) == "" {
			c.seenSizes[key] = size
		}
		// 已存在 slot：仅在尚未缓存 b64 时补齐（多帧/重复事件场景）。
		if b64Payload != "" && c.seenBase64[key] == "" {
			c.seenBase64[key] = b64Payload
		}
		if urlPayload != "" && c.seenURLs[key] == "" {
			c.seenURLs[key] = urlPayload
		}
		return
	}
	c.seen[key] = struct{}{}
	c.seenOrder = append(c.seenOrder, key)
	if size != "" {
		c.seenSizes[key] = size
	}
	if b64Payload != "" {
		c.seenBase64[key] = b64Payload
	}
	if urlPayload != "" {
		c.seenURLs[key] = urlPayload
	}
	c.count++
}

func hashOpenAIImageOutputResult(result string) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(result))
	return hex.EncodeToString(sum[:])
}

func countOpenAIResponseImageOutputsFromJSONBytes(body []byte) int {
	counter := newOpenAIImageOutputCounter()
	counter.AddJSONResponse(body)
	return counter.Count()
}

func collectOpenAIResponseImageOutputSizesFromJSONBytes(body []byte) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddJSONResponse(body)
	return counter.Sizes()
}

func collectOpenAIResponseImageOutputBase64sFromJSONBytes(body []byte) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddJSONResponse(body)
	return counter.Base64Payloads()
}

func collectOpenAIResponseImageOutputURLsFromJSONBytes(body []byte) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddJSONResponse(body)
	return counter.URLs()
}

func countOpenAIImageOutputsFromSSEBody(body string) int {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.Count()
}

func collectOpenAIImageOutputSizesFromSSEBody(body string) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.Sizes()
}

func collectOpenAIImageOutputBase64sFromSSEBody(body string) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.Base64Payloads()
}

func collectOpenAIImageOutputURLsFromSSEBody(body string) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.URLs()
}

type openAIResponseTextOutputCollector struct {
	finalTexts    []string
	fallbackOrder []string
	fallbackTexts map[string]string
	deltaOrder    []string
	deltaTexts    map[string]string
}

func newOpenAIResponseTextOutputCollector() *openAIResponseTextOutputCollector {
	return &openAIResponseTextOutputCollector{
		fallbackTexts: make(map[string]string),
		deltaTexts:    make(map[string]string),
	}
}

func (c *openAIResponseTextOutputCollector) Texts() []string {
	if c == nil {
		return nil
	}
	if len(c.finalTexts) > 0 {
		return cloneNonEmptyStrings(c.finalTexts)
	}
	if len(c.fallbackOrder) > 0 {
		texts := make([]string, 0, len(c.fallbackOrder))
		for _, key := range c.fallbackOrder {
			texts = append(texts, c.fallbackTexts[key])
		}
		return cloneNonEmptyStrings(texts)
	}
	if len(c.deltaOrder) == 0 {
		return nil
	}
	texts := make([]string, 0, len(c.deltaOrder))
	for _, key := range c.deltaOrder {
		texts = append(texts, c.deltaTexts[key])
	}
	return cloneNonEmptyStrings(texts)
}

func (c *openAIResponseTextOutputCollector) AddJSONResponse(body []byte) {
	if c == nil || len(body) == 0 || !gjson.ValidBytes(body) {
		return
	}
	root := gjson.ParseBytes(body)
	texts := collectOpenAIResponseTextsFromRoot(root)
	if len(texts) > 0 {
		c.finalTexts = texts
	}
}

func (c *openAIResponseTextOutputCollector) AddSSEData(data []byte) {
	if c == nil || len(data) == 0 || strings.TrimSpace(string(data)) == "[DONE]" || !gjson.ValidBytes(data) {
		return
	}
	root := gjson.ParseBytes(data)
	eventType := strings.TrimSpace(root.Get("type").String())
	switch eventType {
	case "response.completed", "response.done":
		if texts := collectOpenAIResponseTextsFromRoot(root.Get("response")); len(texts) > 0 {
			c.finalTexts = texts
		}
	case "response.output_item.done":
		c.addOutputItemTexts(root.Get("item"), openAIResponseTextOutputItemPrefix(root.Get("item"), "item"))
	case "response.output_text.done":
		key := openAIResponseTextEventKey(root)
		c.setFallbackText(key, root.Get("text").String())
	case "response.output_text.delta":
		key := openAIResponseTextEventKey(root)
		c.appendDeltaText(key, root.Get("delta").String())
	}
}

func (c *openAIResponseTextOutputCollector) AddSSEBody(body string) {
	if c == nil || strings.TrimSpace(body) == "" {
		return
	}
	forEachOpenAISSEDataPayload(body, c.AddSSEData)
}

func (c *openAIResponseTextOutputCollector) addOutputArrayTexts(output gjson.Result, prefix string) {
	if c == nil || !output.IsArray() {
		return
	}
	output.ForEach(func(index, item gjson.Result) bool {
		c.addOutputItemTexts(item, openAIResponseTextOutputItemPrefix(item, prefix+"."+index.String()))
		return true
	})
}

func (c *openAIResponseTextOutputCollector) addOutputItemTexts(item gjson.Result, prefix string) {
	if c == nil || !item.Exists() || !item.IsObject() {
		return
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	switch itemType {
	case "message", "":
		content := item.Get("content")
		if !content.IsArray() {
			return
		}
		content.ForEach(func(index, part gjson.Result) bool {
			partType := strings.TrimSpace(part.Get("type").String())
			if partType == "" || partType == "output_text" || partType == "text" {
				c.setFallbackText(prefix+"."+index.String(), part.Get("text").String())
			}
			return true
		})
	case "output_text", "text":
		c.setFallbackText(prefix, item.Get("text").String())
	}
}

func (c *openAIResponseTextOutputCollector) setFallbackText(key, text string) {
	if c == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = hashOpenAIImageOutputResult(text)
	}
	if key == "" {
		return
	}
	if _, exists := c.fallbackTexts[key]; !exists {
		c.fallbackOrder = append(c.fallbackOrder, key)
	}
	c.fallbackTexts[key] = text
}

func (c *openAIResponseTextOutputCollector) appendDeltaText(key, delta string) {
	if c == nil {
		return
	}
	if delta == "" {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = strconv.Itoa(len(c.deltaOrder))
	}
	if _, exists := c.deltaTexts[key]; !exists {
		c.deltaOrder = append(c.deltaOrder, key)
	}
	c.deltaTexts[key] += delta
}

func openAIResponseTextEventKey(root gjson.Result) string {
	itemID := strings.TrimSpace(root.Get("item_id").String())
	contentIndex := strings.TrimSpace(root.Get("content_index").String())
	if itemID != "" {
		if contentIndex != "" {
			return itemID + "." + contentIndex
		}
		return itemID
	}
	key := strings.Join([]string{
		strings.TrimSpace(root.Get("output_index").String()),
		contentIndex,
	}, ".")
	key = strings.Trim(key, ".")
	return strings.TrimSpace(key)
}

func openAIResponseTextOutputItemPrefix(item gjson.Result, fallback string) string {
	if item.Exists() {
		if itemID := strings.TrimSpace(item.Get("id").String()); itemID != "" {
			return itemID
		}
		if itemID := strings.TrimSpace(item.Get("call_id").String()); itemID != "" {
			return itemID
		}
	}
	return strings.TrimSpace(fallback)
}

func collectOpenAIResponseTextsFromRoot(root gjson.Result) []string {
	if !root.Exists() || !root.IsObject() {
		return nil
	}
	collector := newOpenAIResponseTextOutputCollector()
	collector.addOutputArrayTexts(root.Get("output"), "output")
	collector.addOutputArrayTexts(root.Get("response.output"), "response.output")
	texts := collector.Texts()
	if len(texts) > 0 {
		return texts
	}
	if text := strings.TrimSpace(root.Get("output_text").String()); text != "" {
		return []string{text}
	}
	if text := strings.TrimSpace(root.Get("response.output_text").String()); text != "" {
		return []string{text}
	}
	return nil
}

func collectOpenAIResponseOutputTextsFromJSONBytes(body []byte) []string {
	collector := newOpenAIResponseTextOutputCollector()
	collector.AddJSONResponse(body)
	return collector.Texts()
}

func collectOpenAIResponseOutputTextsFromSSEBody(body string) []string {
	collector := newOpenAIResponseTextOutputCollector()
	collector.AddSSEBody(body)
	return collector.Texts()
}
