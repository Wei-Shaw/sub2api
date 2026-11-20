package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ResponsesInputHasFunctionCallOutputNamed reports whether the top-level input
// array holds an object item whose type is "function_call_output" and whose
// name starts with one of namePrefixes. Keys and string values are compared
// after JSON unescaping, through a zero-copy view of body. When a member is
// duplicated, the first occurrence is read; callers that decode the body
// treat duplicate members separately.
func ResponsesInputHasFunctionCallOutputNamed(body []byte, namePrefixes ...string) bool {
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() || item.Get("type").String() != "function_call_output" {
			return true
		}
		name := item.Get("name").String()
		for _, prefix := range namePrefixes {
			if strings.HasPrefix(name, prefix) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
