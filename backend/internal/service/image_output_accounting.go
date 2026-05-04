package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIImageOutputCounter struct {
	seen         map[string]struct{REDACTED
	count        int
	maxDataCount int
REDACTED

func newOpenAIImageOutputCounter() *openAIImageOutputCounter {
	return &openAIImageOutputCounter{seen: make(map[string]struct{REDACTED)REDACTED
REDACTED

func (c *openAIImageOutputCounter) Count() int {
	if c == nil {
		return 0
REDACTED
	if c.maxDataCount > c.count {
		return c.maxDataCount
REDACTED
	return c.count
REDACTED

func (c *openAIImageOutputCounter) AddJSONResponse(body []byte) {
	if c == nil || len(body) == 0 || !gjson.ValidBytes(body) {
		return
REDACTED
	c.addDataArray(gjson.GetBytes(body, "data"))
	c.addOutputArray(gjson.GetBytes(body, "output"))
	c.addOutputArray(gjson.GetBytes(body, "response.output"))
REDACTED

func (c *openAIImageOutputCounter) AddSSEData(data []byte) {
	if c == nil || len(data) == 0 || strings.TrimSpace(string(data)) == "[DONE]" || !gjson.ValidBytes(data) {
		return
REDACTED
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
	REDACTED
		if output := root.Get("output"); output.Exists() {
			c.addImageOutputItem(output)
			return
	REDACTED
		c.addImageOutputItem(root)
REDACTED
REDACTED

func (c *openAIImageOutputCounter) AddSSEBody(body string) {
	if c == nil || strings.TrimSpace(body) == "" {
		return
REDACTED
	forEachOpenAISSEDataPayload(body, c.AddSSEData)
REDACTED

func (c *openAIImageOutputCounter) addDataArray(data gjson.Result) {
	if !data.IsArray() {
		return
REDACTED
	count := len(data.Array())
	if count > c.maxDataCount {
		c.maxDataCount = count
REDACTED
REDACTED

func (c *openAIImageOutputCounter) addOutputArray(output gjson.Result) {
	if !output.IsArray() {
		return
REDACTED
	output.ForEach(func(_, item gjson.Result) bool {
		c.addImageOutputItem(item)
		return true
REDACTED)
REDACTED

func (c *openAIImageOutputCounter) addImageOutputItem(item gjson.Result) {
	if !item.Exists() || !item.IsObject() {
		return
REDACTED
	itemType := strings.TrimSpace(item.Get("type").String())
	if itemType != "" && itemType != "image_generation_call" && itemType != "image_generation.completed" {
		return
REDACTED
	if strings.Contains(strings.ToLower(item.Raw), "partial_image") {
		return
REDACTED
	result := strings.TrimSpace(item.Get("result").String())
	if result == "" {
		result = strings.TrimSpace(item.Get("b64_json").String())
REDACTED
	if result == "" {
		result = strings.TrimSpace(item.Get("url").String())
REDACTED
	if result == "" && itemType != "image_generation.completed" {
		return
REDACTED
	key := strings.TrimSpace(item.Get("id").String())
	if key == "" {
		key = strings.TrimSpace(item.Get("call_id").String())
REDACTED
	if key == "" {
		key = hashOpenAIImageOutputResult(result)
REDACTED
	if key == "" {
		return
REDACTED
	if _, exists := c.seen[key]; exists {
		return
REDACTED
	c.seen[key] = struct{REDACTED{REDACTED
	c.count++
REDACTED

func hashOpenAIImageOutputResult(result string) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return ""
REDACTED
	sum := sha256.Sum256([]byte(result))
	return hex.EncodeToString(sum[:])
REDACTED

func countOpenAIResponseImageOutputsFromJSONBytes(body []byte) int {
	counter := newOpenAIImageOutputCounter()
	counter.AddJSONResponse(body)
	return counter.Count()
REDACTED

func countOpenAIImageOutputsFromSSEBody(body string) int {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.Count()
REDACTED
