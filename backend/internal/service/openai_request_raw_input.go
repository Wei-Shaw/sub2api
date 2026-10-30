package service

import "github.com/tidwall/gjson"

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
