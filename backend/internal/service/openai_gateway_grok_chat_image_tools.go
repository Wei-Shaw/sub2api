package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 当前轮的内联 image_url 已可由 Grok 直接读取；若同时保留客户端本地
// view_image，Grok 可能只预告调用工具而不继续作答，因此只移除这一冗余自动选择。
func stripRedundantGrokChatViewImageTool(body []byte) ([]byte, error) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body, nil
REDACTED
	items := messages.Array()
	if len(items) == 0 {
		return body, nil
REDACTED
	current := items[len(items)-1]
	if strings.TrimSpace(current.Get("role").String()) != "user" ||
		!openAIJSONValueMayContainImageInput(current) {
		return body, nil
REDACTED

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if toolChoice.IsObject() && strings.TrimSpace(toolChoice.Get("type").String()) == "function" {
		choiceName := strings.TrimSpace(toolChoice.Get("function.name").String())
		if choiceName == "" {
			choiceName = strings.TrimSpace(toolChoice.Get("name").String())
	REDACTED
		if choiceName == "view_image" {
			return body, nil
	REDACTED
REDACTED

	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body, nil
REDACTED
	filtered := make([]json.RawMessage, 0, len(tools.Array()))
	changed := false
	for _, tool := range tools.Array() {
		toolName := strings.TrimSpace(tool.Get("function.name").String())
		if toolName == "" {
			toolName = strings.TrimSpace(tool.Get("name").String())
	REDACTED
		if strings.TrimSpace(tool.Get("type").String()) == "function" && toolName == "view_image" {
			changed = true
			continue
	REDACTED
		filtered = append(filtered, json.RawMessage(tool.Raw))
REDACTED
	if !changed {
		return body, nil
REDACTED
	if len(filtered) == 0 && strings.TrimSpace(toolChoice.String()) == "required" {
		return body, nil
REDACTED

	if len(filtered) > 0 {
		encoded, err := json.Marshal(filtered)
		if err != nil {
			return nil, err
	REDACTED
		return sjson.SetRawBytes(body, "tools", encoded)
REDACTED

	out, err := sjson.DeleteBytes(body, "tools")
	if err != nil {
		return nil, err
REDACTED
	out, err = sjson.DeleteBytes(out, "parallel_tool_calls")
	if err != nil {
		return nil, err
REDACTED
	if strings.TrimSpace(toolChoice.String()) == "auto" {
		out, err = sjson.DeleteBytes(out, "tool_choice")
REDACTED
	return out, err
REDACTED
