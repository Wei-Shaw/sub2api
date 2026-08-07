package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// countGrokNativeSearchCallsFromJSONBytes counts completed native search tool
// calls in a Responses-style JSON body (output array or nested response.output).
// Counts: web_search_call, x_search_call, tool_search_call, and function_call
// named tool_search / web_search / x_search.
func countGrokNativeSearchCallsFromJSONBytes(body []byte) int {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0
REDACTED
	count := 0
	count += countGrokNativeSearchCallsInOutputArray(gjson.GetBytes(body, "output"))
	count += countGrokNativeSearchCallsInOutputArray(gjson.GetBytes(body, "response.output"))
	return count
REDACTED

func countGrokNativeSearchCallsFromSSEBody(body string) int {
	if strings.TrimSpace(body) == "" {
		return 0
REDACTED
	seen := make(map[string]struct{REDACTED)
	total := 0
	forEachOpenAISSEDataPayload(body, func(data []byte) {
		total += countGrokNativeSearchCallsInSSEDataDedup(data, seen)
REDACTED)
	return total
REDACTED

// countGrokNativeSearchCallsInSSEData counts search tool calls in one SSE
// payload without cross-event dedup. Prefer countGrokNativeSearchCallsInSSEDataDedup
// for live streams so item.done + response.completed do not double-bill.
func countGrokNativeSearchCallsInSSEData(data []byte) int {
	n, _ := countGrokNativeSearchCallsInSSEDataWithKeys(data)
	return n
REDACTED

// countGrokNativeSearchCallsInSSEDataDedup increments only unseen call ids.
// Callers must reuse the same seen map for the full stream lifetime.
func countGrokNativeSearchCallsInSSEDataDedup(data []byte, seen map[string]struct{REDACTED) int {
	if seen == nil {
		return countGrokNativeSearchCallsInSSEData(data)
REDACTED
	n, keys := countGrokNativeSearchCallsInSSEDataWithKeys(data)
	if n <= 0 {
		return 0
REDACTED
	// No stable id: fall back to raw count once (cannot dedup across events).
	if len(keys) == 0 {
		return n
REDACTED
	// Intra-event + cross-event dedup by call_id/id.
	added := 0
	local := make(map[string]struct{REDACTED, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
	REDACTED
		if _, ok := local[k]; ok {
			continue
	REDACTED
		local[k] = struct{REDACTED{REDACTED
		if _, ok := seen[k]; ok {
			continue
	REDACTED
		seen[k] = struct{REDACTED{REDACTED
		added++
REDACTED
	return added
REDACTED

func countGrokNativeSearchCallsInSSEDataWithKeys(data []byte) (int, []string) {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return 0, nil
REDACTED
	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	// Count once on item completion / completed response, not on every delta.
	switch eventType {
	case "response.output_item.done", "response.completed", "response.done":
	default:
		// Also allow bare item objects without envelope.
		if eventType != "" {
			return 0, nil
	REDACTED
REDACTED
	var keys []string
	n := 0
	consider := func(item gjson.Result) {
		if !isGrokNativeSearchOutputItem(item) {
			return
	REDACTED
		n++
		key := firstNonEmpty(
			strings.TrimSpace(item.Get("call_id").String()),
			strings.TrimSpace(item.Get("id").String()),
			strings.TrimSpace(item.Get("item.call_id").String()),
			strings.TrimSpace(item.Get("item.id").String()),
		)
		if key != "" {
			keys = append(keys, key)
	REDACTED
REDACTED
	if item := gjson.GetBytes(data, "item"); item.Exists() {
		consider(item)
REDACTED
	gjson.GetBytes(data, "response.output").ForEach(func(_, item gjson.Result) bool {
		consider(item)
		return true
REDACTED)
	gjson.GetBytes(data, "output").ForEach(func(_, item gjson.Result) bool {
		consider(item)
		return true
REDACTED)
	// Bare output item event without nested item key.
	if n == 0 && isGrokNativeSearchOutputItem(gjson.ParseBytes(data)) {
		consider(gjson.ParseBytes(data))
REDACTED
	return n, keys
REDACTED

func countGrokNativeSearchCallsInOutputArray(output gjson.Result) int {
	if !output.IsArray() {
		return 0
REDACTED
	count := 0
	output.ForEach(func(_, item gjson.Result) bool {
		if isGrokNativeSearchOutputItem(item) {
			count++
	REDACTED
		return true
REDACTED)
	return count
REDACTED

func isGrokNativeSearchOutputItem(item gjson.Result) bool {
	if !item.Exists() {
		return false
REDACTED
	itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	switch itemType {
	case "web_search_call", "x_search_call", "tool_search_call":
		return true
	case "function_call", "custom_tool_call":
		name := strings.ToLower(strings.TrimSpace(item.Get("name").String()))
		return name == "web_search" || name == "x_search" || name == "tool_search"
	default:
		return false
REDACTED
REDACTED
