package handler

import "github.com/tidwall/gjson"

const invalidStreamFieldTypeMessage = "invalid stream field type"

func parseOpenAICompatibleStream(body []byte) (bool, bool) {
	streamResult := gjson.GetBytes(body, "stream")
	if streamResult.Exists() && streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		return false, false
	}
	return streamResult.Bool(), true
}
