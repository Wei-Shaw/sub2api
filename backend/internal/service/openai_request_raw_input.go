package service

import (
	"cmp"
	"errors"
	"slices"

	"github.com/tidwall/gjson"
)

// rawJSONValueEdit replaces value, a result that gjson located in the body
// being edited, with raw.
type rawJSONValueEdit struct {
	value gjson.Result
	raw   []byte
}

// spliceRawJSONValues writes body with every edit applied into one new slice,
// so several small rewrites do not each copy a large body. The values must be
// located in body and must not overlap; body itself is not modified.
func spliceRawJSONValues(body []byte, edits []rawJSONValueEdit) ([]byte, error) {
	slices.SortFunc(edits, func(a, b rawJSONValueEdit) int { return cmp.Compare(a.value.Index, b.value.Index) })
	size := len(body)
	end := 0
	for _, edit := range edits {
		start := edit.value.Index
		if start <= 0 || start < end || start+len(edit.value.Raw) > len(body) || string(body[start:start+len(edit.value.Raw)]) != edit.value.Raw {
			return nil, errors.New("value is not located in body")
		}
		end = start + len(edit.value.Raw)
		size += len(edit.raw) - len(edit.value.Raw)
	}
	result := make([]byte, 0, size)
	end = 0
	for _, edit := range edits {
		result = append(result, body[end:edit.value.Index]...)
		result = append(result, edit.raw...)
		end = edit.value.Index + len(edit.value.Raw)
	}
	return append(result, body[end:]...), nil
}

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

func skipRawJSONSpace(raw string, i int) int {
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

// rawJSONStringEnd returns the index just past the string literal that starts
// at raw[i], without decoding it.
func rawJSONStringEnd(raw string, i int) (int, bool) {
	if i >= len(raw) || raw[i] != '"' {
		return i, false
	}
	for i++; i < len(raw); i++ {
		switch raw[i] {
		case '\\':
			i++
		case '"':
			return i + 1, true
		}
	}
	return len(raw), false
}

// rawJSONValueEnd returns the index just past the value that starts at raw[i],
// without decoding it. It only finds the extent of a value in JSON that has
// already been validated; it does not validate literals or numbers.
func rawJSONValueEnd(raw string, i int) (int, bool) {
	if i >= len(raw) {
		return i, false
	}
	switch raw[i] {
	case '"':
		return rawJSONStringEnd(raw, i)
	case '{', '[':
		depth := 0
		for ; i < len(raw); i++ {
			switch raw[i] {
			case '"':
				end, ok := rawJSONStringEnd(raw, i)
				if !ok {
					return end, false
				}
				i = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, true
				}
			}
		}
		return len(raw), false
	default:
		start := i
		for i < len(raw) {
			switch raw[i] {
			case ',', '}', ']', ' ', '\t', '\r', '\n':
				return i, i > start
			}
			i++
		}
		return i, i > start
	}
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
