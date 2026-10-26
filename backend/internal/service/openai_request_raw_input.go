package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// replaceOpenAIRawInput copies unchanged image strings only into the final
// request, without allocating a second complete input array first.
func replaceOpenAIRawInput(body []byte, input gjson.Result, items []string) []byte {
	size := len(body) - len(input.Raw) + 2
	for index, item := range items {
		size += len(item)
		if index > 0 {
			size++
		}
	}
	result := make([]byte, 0, size)
	result = append(result, body[:input.Index]...)
	result = append(result, '[')
	for index, item := range items {
		if index > 0 {
			result = append(result, ',')
		}
		result = append(result, item...)
	}
	result = append(result, ']')
	return append(result, body[input.Index+len(input.Raw):]...)
}

// The standard decoder keeps the last duplicate key; GJSON selects the first.
func hasDuplicateJSONObjectKeys(object gjson.Result) bool {
	seen := make(map[string]struct{})
	duplicate := false
	object.ForEach(func(key, _ gjson.Result) bool {
		if _, exists := seen[key.Str]; exists {
			duplicate = true
			return false
		}
		seen[key.Str] = struct{}{}
		return true
	})
	return duplicate
}

// ValidateOpenAICompactionInputShape rejects member collisions that would make
// GJSON and encoding/json disagree about which compaction state is present.
func ValidateOpenAICompactionInputShape(body []byte) error {
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return nil
	}
	inputKeys := 0
	root.ForEach(func(key, _ gjson.Result) bool {
		if strings.EqualFold(key.Str, "input") {
			if key.Str != "input" {
				return false
			}
			inputKeys++
		}
		return true
	})
	ambiguousInput := false
	root.ForEach(func(key, _ gjson.Result) bool {
		if strings.EqualFold(key.Str, "input") && key.Str != "input" {
			ambiguousInput = true
			return false
		}
		return true
	})
	if ambiguousInput {
		return fmt.Errorf("%w: input member spelling must be canonical", ErrOpenAICompactionSummaryInvalid)
	}
	if inputKeys > 1 {
		return fmt.Errorf("%w: duplicate input member", ErrOpenAICompactionSummaryInvalid)
	}
	input := root.Get("input")
	checkItem := func(item gjson.Result) error {
		if !item.IsObject() {
			return nil
		}
		for _, member := range []string{"type", "encrypted_content", "id"} {
			count := 0
			ambiguous := false
			item.ForEach(func(key, _ gjson.Result) bool {
				if strings.EqualFold(key.Str, member) {
					if key.Str != member {
						ambiguous = true
						return false
					}
					count++
				}
				return true
			})
			if ambiguous {
				return fmt.Errorf("%w: input item member %s has ambiguous spelling", ErrOpenAICompactionSummaryInvalid, member)
			}
			if count > 1 {
				return fmt.Errorf("%w: duplicate input item member %s", ErrOpenAICompactionSummaryInvalid, member)
			}
		}
		return nil
	}
	if input.IsArray() {
		var itemErr error
		input.ForEach(func(_, item gjson.Result) bool {
			if err := checkItem(item); err != nil {
				itemErr = err
				return false
			}
			return true
		})
		return itemErr
	}
	if input.IsObject() {
		return checkItem(input)
	}
	return nil
}

// ValidateOpenAIWSFrameTypeShape prevents GJSON and the downstream decoder from
// routing different event types when member spelling or duplicate keys differ.
func ValidateOpenAIWSFrameTypeShape(body []byte) error {
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return nil
	}
	count := 0
	ambiguous := false
	root.ForEach(func(key, _ gjson.Result) bool {
		if strings.EqualFold(key.Str, "type") {
			if key.Str != "type" {
				ambiguous = true
				return false
			}
			count++
		}
		return true
	})
	if ambiguous {
		return fmt.Errorf("websocket event type member spelling must be canonical")
	}
	if count > 1 {
		return fmt.Errorf("websocket event type member must be unique")
	}
	return nil
}
