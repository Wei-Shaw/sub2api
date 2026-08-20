package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func openAIResponsesInputItemIDPrefix(itemType string) (string, bool) {
	switch strings.TrimSpace(itemType) {
	case "message":
		return "msg", true
	case "reasoning":
		return "rs", true
	case "web_search_call":
		return "ws", true
	case "custom_tool_call":
		return openAIResponsesToolCallIDPrefix(itemType), true
	case "tool_search_call":
		return openAIResponsesToolCallIDPrefix(itemType), true
	case "custom_tool_call_output":
		// Although custom calls use ctc IDs, OpenAI validates replayed custom
		// call output item IDs against the generic fc namespace.
		return "fc", true
	default:
		if isCodexToolCallInputType(itemType) {
			return openAIResponsesToolCallIDPrefix(itemType), true
	REDACTED
		return "", false
REDACTED
REDACTED

func openAIResponsesToolCallIDPrefix(itemType string) string {
	switch strings.TrimSpace(itemType) {
	case "custom_tool_call", "custom_tool_call_output":
		return "ctc"
	case "tool_search_call", "tool_search_output":
		return "tsc"
	default:
		return "fc"
REDACTED
REDACTED

// Invalid replayed IDs are removed rather than rewritten because a fabricated
// ID may point at a different upstream object.
func shouldStripOpenAIResponsesInputItemID(itemType, id string) bool {
	prefix, constrained := openAIResponsesInputItemIDPrefix(itemType)
	if !constrained {
		return false
REDACTED
	return id == "" || !strings.HasPrefix(id, prefix)
REDACTED

func shouldStripOpenAIResponsesNonPairCallID(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "message", "reasoning", "image_generation_call":
		return true
	default:
		return false
REDACTED
REDACTED

func sanitizeOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
REDACTED

	type inputItem struct {
		body        []byte
		itemType    string
		id          string
		callID      string
		stripID     bool
		stripCallID bool
		drop        bool
		isObject    bool
REDACTED

	items := make([]inputItem, 0)
	strippedIDs := make(map[string]struct{REDACTED)
	validCallIDs := make(map[string]struct{REDACTED)
	input.ForEach(func(_, item gjson.Result) bool {
		parsed := inputItem{body: []byte(item.Raw), isObject: item.IsObject()REDACTED
		if item.IsObject() {
			itemType := item.Get("type")
			id := item.Get("id")
			parsed.itemType = strings.TrimSpace(itemType.String())
			parsed.callID = strings.TrimSpace(item.Get("call_id").String())
			parsed.stripCallID = item.Get("call_id").Exists() && shouldStripOpenAIResponsesNonPairCallID(parsed.itemType)
			if id.Type == gjson.String {
				parsed.id = id.String()
				parsed.stripID = shouldStripOpenAIResponsesInputItemID(parsed.itemType, parsed.id)
				if parsed.stripID && parsed.id != "" {
					strippedIDs[parsed.id] = struct{REDACTED{REDACTED
			REDACTED
		REDACTED
			if isCodexToolCallContextItemType(parsed.itemType) && parsed.callID != "" {
				validCallIDs[parsed.callID] = struct{REDACTED{REDACTED
		REDACTED
	REDACTED
		items = append(items, parsed)
		return true
REDACTED)
	hasSanitization := false
	for _, item := range items {
		if item.stripID || item.stripCallID {
			hasSanitization = true
			break
	REDACTED
REDACTED
	if !hasSanitization {
		return body, false, nil
REDACTED

	// First decide which outputs become dangling because their call_id points at
	// an item ID that is being removed and no call item owns that call_id. This
	// must happen before computing retained IDs: a dropped output cannot keep an
	// item_reference alive merely because it used to have the same id.
	for index := range items {
		item := &items[index]
		if !item.isObject || !isCodexToolCallOutputItemType(item.itemType) {
			continue
	REDACTED
		if _, pointsAtStrippedID := strippedIDs[item.callID]; !pointsAtStrippedID {
			continue
	REDACTED
		if _, hasMatchingCallID := validCallIDs[item.callID]; !hasMatchingCallID {
			item.drop = true
	REDACTED
REDACTED

	removedItemIDs := make(map[string]struct{REDACTED, len(strippedIDs))
	retainedItemIDs := make(map[string]struct{REDACTED, len(items))
	for _, item := range items {
		if item.id == "" || item.itemType == "item_reference" {
			continue
	REDACTED
		if item.stripID || item.drop {
			removedItemIDs[item.id] = struct{REDACTED{REDACTED
			continue
	REDACTED
		retainedItemIDs[item.id] = struct{REDACTED{REDACTED
REDACTED
	for id := range retainedItemIDs {
		delete(removedItemIDs, id)
REDACTED

	rebuiltItems := make([][]byte, 0, len(items))
	for index, item := range items {
		if item.isObject {
			if item.itemType == "item_reference" {
				if _, dangling := removedItemIDs[item.id]; dangling {
					continue
			REDACTED
		REDACTED
			if item.drop {
				continue
		REDACTED
	REDACTED
		itemBody := item.body
		if item.stripID {
			var err error
			itemBody, err = sjson.DeleteBytes(itemBody, "id")
			if err != nil {
				return nil, false, fmt.Errorf("delete input.%d.id: %w", index, err)
		REDACTED
	REDACTED
		if item.stripCallID {
			var err error
			itemBody, err = sjson.DeleteBytes(itemBody, "call_id")
			if err != nil {
				return nil, false, fmt.Errorf("delete input.%d.call_id: %w", index, err)
		REDACTED
	REDACTED
		rebuiltItems = append(rebuiltItems, itemBody)
REDACTED

	rebuiltInput := make([]byte, 0, len(input.Raw))
	rebuiltInput = append(rebuiltInput, '[')
	for i, item := range rebuiltItems {
		if i > 0 {
			rebuiltInput = append(rebuiltInput, ',')
	REDACTED
		rebuiltInput = append(rebuiltInput, item...)
REDACTED
	rebuiltInput = append(rebuiltInput, ']')

	sanitized, err := sjson.SetRawBytes(body, "input", rebuiltInput)
	if err != nil {
		return nil, false, fmt.Errorf("replace sanitized input: %w", err)
REDACTED
	return sanitized, true, nil
REDACTED
