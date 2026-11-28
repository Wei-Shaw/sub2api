package service

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// applyGeminiChatResponseFormat 把 Chat Completions 的 response_format 写入 Gemini generationConfig：
// json_object 写 responseMimeType=application/json；json_schema 另把 json_schema.schema 原样写入
// responseJsonSchema（JSON Schema 形态，不转换为 OpenAPI 子集的 responseSchema）。
// text、缺省或无法解析的 response_format 不改请求。
// 以下情况不写：请求带 tools（Gemini 2.x 不接受工具与 JSON responseMimeType 同用）；
// upstreamModel 为图片输出模型（输出含图片，与 JSON responseMimeType 冲突）。
func applyGeminiChatResponseFormat(geminiReq []byte, responseFormat json.RawMessage, upstreamModel string) []byte {
	if len(responseFormat) == 0 || !gjson.ValidBytes(responseFormat) || isGeminiImageOutputModel(upstreamModel) {
		return geminiReq
	}
	format := gjson.ParseBytes(responseFormat)
	if !format.IsObject() {
		return geminiReq
	}

	var schema gjson.Result
	switch format.Get("type").String() {
	case "json_object":
	case "json_schema":
		schema = format.Get("json_schema.schema")
	default:
		return geminiReq
	}

	if tools := gjson.GetBytes(geminiReq, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		return geminiReq
	}

	out, err := sjson.SetBytes(geminiReq, "generationConfig.responseMimeType", "application/json")
	if err != nil {
		return geminiReq
	}
	if schema.IsObject() {
		withSchema, err := sjson.SetRawBytes(out, "generationConfig.responseJsonSchema", []byte(schema.Raw))
		if err != nil {
			return geminiReq
		}
		out = withSchema
	}
	return out
}
