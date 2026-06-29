package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIImageOutputCounter struct {
	seen         map[string]struct{REDACTED
	seenSizes    map[string]string
	seenOrder    []string
	dataSizes    []string
	count        int
	maxDataCount int
REDACTED

func newOpenAIImageOutputCounter() *openAIImageOutputCounter {
	return &openAIImageOutputCounter{
		seen:      make(map[string]struct{REDACTED),
		seenSizes: make(map[string]string),
REDACTED
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

func (c *openAIImageOutputCounter) Sizes() []string {
	if c == nil {
		return nil
REDACTED
	sizes := make([]string, 0, len(c.seenOrder)+len(c.dataSizes))
	for _, key := range c.seenOrder {
		if size := strings.TrimSpace(c.seenSizes[key]); size != "" {
			sizes = append(sizes, size)
	REDACTED
REDACTED
	if len(sizes) == 0 && len(c.dataSizes) > 0 {
		sizes = append(sizes, c.dataSizes...)
REDACTED
	if len(sizes) == 0 {
		return nil
REDACTED
	return sizes
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
	items := data.Array()
	imageCount := 0
	sizes := make([]string, 0, len(items))
	for _, item := range items {
		if !item.IsObject() {
			continue
	REDACTED
		hasImageOutput := strings.TrimSpace(item.Get("url").String()) != "" ||
			strings.TrimSpace(item.Get("b64_json").String()) != ""
		if !hasImageOutput {
			continue
	REDACTED
		imageCount++
		if size := strings.TrimSpace(item.Get("size").String()); size != "" {
			sizes = append(sizes, size)
	REDACTED
REDACTED
	if imageCount > c.maxDataCount {
		c.maxDataCount = imageCount
REDACTED
	if len(sizes) > 0 {
		c.dataSizes = sizes
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
	if result == "" {
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
	size := strings.TrimSpace(item.Get("size").String())
	if _, exists := c.seen[key]; exists {
		if size != "" && strings.TrimSpace(c.seenSizes[key]) == "" {
			c.seenSizes[key] = size
	REDACTED
		return
REDACTED
	c.seen[key] = struct{REDACTED{REDACTED
	c.seenOrder = append(c.seenOrder, key)
	if size != "" {
		c.seenSizes[key] = size
REDACTED
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

func collectOpenAIResponseImageOutputSizesFromJSONBytes(body []byte) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddJSONResponse(body)
	return counter.Sizes()
REDACTED

func countOpenAIImageOutputsFromSSEBody(body string) int {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.Count()
REDACTED

func collectOpenAIImageOutputSizesFromSSEBody(body string) []string {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(body)
	return counter.Sizes()
REDACTED
