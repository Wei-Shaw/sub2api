package handler

import "github.com/tidwall/gjson"

// Leave question schemas and model support to the upstream so new native
// fields and model aliases remain usable without a gateway upgrade.
func validateOpenAIDecisionsRequest(body []byte) string {
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return "Request body must be a JSON object"
	}
	input := root.Get("input")
	if input.Type != gjson.String && !input.IsArray() {
		return "input must be a string or an array"
	}
	questions := root.Get("questions")
	if !questions.IsArray() || len(questions.Array()) == 0 || len(questions.Array()) > 400 {
		return "questions must be an array containing between 1 and 400 questions"
	}
	if stream := root.Get("stream"); stream.Exists() && stream.Type != gjson.False {
		return "Decisions API does not support streaming"
	}
	return ""
}
