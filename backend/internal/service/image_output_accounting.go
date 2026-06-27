package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIImageOutputCounter struct {
	seen         map[string]struct{}
	seenSizes    map[string]string
	seenBase64   map[string]string
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
	count := len(items)
	if count > c.maxDataCount {
		c.maxDataCount = count
	}
	sizes := make([]string, 0, len(items))
	for _, item := range items {
		if size := strings.TrimSpace(item.Get("size").String()); size != "" {
			sizes = append(sizes, size)
		}
		// 修复：对 data 数组中的每个项目也调用 addImageOutputItem 来处理 b64_json 字段
		c.addImageOutputItem(item)
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
	if result == "" && itemType != "image_generation.completed" {
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
