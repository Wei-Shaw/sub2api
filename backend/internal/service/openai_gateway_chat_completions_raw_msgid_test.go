//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestEnsureChatMessagesIDs(t *testing.T) {
	t.Parallel()

	// 输入：3 条消息，中间一条已带 id，首尾缺 id（含一条多模态 content）
	body := []byte(`{
		"model": "deepseek-v4-flash",
		"messages": [
			{"role": "user", "content": "q1"},
			{"role": "assistant", "content": "a1", "id": "keep-this-id"},
			{"role": "user", "content": [
				{"type": "text", "text": "看这张图"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,AAA"}}
			]}
		]
	}`)

	out, err := ensureChatMessagesIDs(body)
	if err != nil {
		t.Fatalf("ensureChatMessagesIDs error: %v", err)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON: %s", out)
	}

	// 已有 id 的消息保留原值
	if got := gjson.GetBytes(out, "messages.1.id").String(); got != "keep-this-id" {
		t.Fatalf("expected existing id preserved, got %q", got)
	}

	// 缺 id 的消息补齐
	for i := 0; i < 3; i++ {
		id := gjson.GetBytes(out, "messages."+string(rune('0'+i))+".id").String()
		if id == "" {
			t.Fatalf("messages[%d] missing id", i)
		}
	}

	// 无 messages 的 body 原样返回
	plain := []byte(`{"model":"x"}`)
	out2, err := ensureChatMessagesIDs(plain)
	if err != nil {
		t.Fatalf("ensureChatMessagesIDs plain error: %v", err)
	}
	if string(out2) != string(plain) {
		t.Fatalf("plain body changed: %s", out2)
	}

	// 空 messages 原样返回
	empty := []byte(`{"messages":[]}`)
	out3, err := ensureChatMessagesIDs(empty)
	if err != nil {
		t.Fatalf("ensureChatMessagesIDs empty error: %v", err)
	}
	if string(out3) != string(empty) {
		t.Fatalf("empty body changed: %s", out3)
	}
}
