package codebuddy

import (
	"strings"
	"testing"
	"time"
)

func TestBuildURLs(t *testing.T) {
	if got := BuildAuthStateURL(DefaultBaseURL); got != "https://copilot.tencent.com/v2/plugin/auth/state" {
		t.Fatalf("BuildAuthStateURL = %q", got)
	}
	if got := BuildAuthTokenURL(DefaultBaseURL, "abc"); got != "https://copilot.tencent.com/v2/plugin/auth/token?state=abc" {
		t.Fatalf("BuildAuthTokenURL = %q", got)
	}
	if got := BuildAuthTokenRefreshURL(DefaultBaseURL); got != "https://copilot.tencent.com/v2/plugin/auth/token/refresh" {
		t.Fatalf("BuildAuthTokenRefreshURL = %q", got)
	}
	if got := BuildLoginAccountURL(DefaultBaseURL, "abc"); got != "https://copilot.tencent.com/v2/plugin/login/account?state=abc" {
		t.Fatalf("BuildLoginAccountURL = %q", got)
	}
	if got := BuildConfigURL(DefaultBaseURL); got != "https://copilot.tencent.com/v3/config" {
		t.Fatalf("BuildConfigURL = %q", got)
	}
	chat, err := BuildChatCompletionsURL(DefaultBaseURL)
	if err != nil || chat != "https://copilot.tencent.com/v2/chat/completions" {
		t.Fatalf("BuildChatCompletionsURL = %q, err=%v", chat, err)
	}
	// 自定义 base_url 应被规范化（去掉尾部斜杠）。
	chat2, err := BuildChatCompletionsURL("https://example.com/")
	if err != nil || !strings.HasSuffix(chat2, "/v2/chat/completions") {
		t.Fatalf("BuildChatCompletionsURL custom = %q, err=%v", chat2, err)
	}
}

func TestParseEnabledModels(t *testing.T) {
	body := []byte(`{
		"code":0,
		"data":{
			"agents":[
				{"name":"cli","models":["auto","hy3","glm-5.2","hy3","deepseek-v4-flash"]},
				{"name":"craft","models":["glm-5.1","auto","kimi-k2.7"]}
			]
		}
	}`)
	models, err := ParseEnabledModels(body)
	if err != nil {
		t.Fatalf("ParseEnabledModels err = %v", err)
	}
	want := []string{"deepseek-v4-flash", "glm-5.1", "glm-5.2", "hy3", "kimi-k2.7"}
	if len(models) != len(want) {
		t.Fatalf("got %v, want %v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("models[%d] = %q, want %q (full=%v)", i, models[i], want[i], models)
		}
	}
}

func TestParseEnabledModelsInvalidJSON(t *testing.T) {
	if _, err := ParseEnabledModels([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestRuntimeSanity(t *testing.T) {
	r := RuntimeSanity()
	if !strings.Contains(r.ChatCompletions, "/v2/chat/completions") {
		t.Fatalf("RuntimeSanity chat url = %q", r.ChatCompletions)
	}
	if !strings.Contains(r.AuthState, "platform=workbuddy") {
		t.Fatalf("RuntimeSanity auth state = %q", r.AuthState)
	}
}

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	sid := "sess1"
	store.Set(sid, &OAuthSession{State: "st", CreatedAt: time.Now()})
	if s, ok := store.Get(sid); !ok || s.State != "st" {
		t.Fatalf("Get returned ok=%v", ok)
	}
	store.Delete(sid)
	if _, ok := store.Get(sid); ok {
		t.Fatal("expected session deleted")
	}
}
