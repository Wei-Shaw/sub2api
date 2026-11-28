package service

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestRequestBodyHasImageInput(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "anthropic image block (base64)",
			body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}},{"type":"text","text":"describe"}]}]}`,
			want: true,
		},
		{
			name: "anthropic image block (url)",
			body: `{"model":"m","messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.com/x.png"}}]}]}`,
			want: true,
		},
		{
			name: "openai chat completions image_url",
			body: `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}]}]}`,
			want: true,
		},
		{
			name: "openai responses input_image",
			body: `{"model":"m","input":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}]}`,
			want: true,
		},
		{
			name: "image inside tool_result content",
			body: `{"model":"m","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"image","source":{"type":"base64","data":"aGVsbG8="}}]}]}]}`,
			want: true,
		},
		{
			name: "plain text anthropic",
			body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hello"}]}`,
			want: false,
		},
		{
			name: "plain text openai responses",
			body: `{"model":"m","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`,
			want: false,
		},
		{
			name: "type marker without image payload is ignored",
			body: `{"model":"m","messages":[{"role":"user","content":[{"type":"image"}]}]}`,
			want: false,
		},
		{
			name: "type marker case-insensitive",
			body: `{"model":"m","messages":[{"role":"user","content":[{"type":"IMAGE_URL","image_url":{"url":"data:x"}}]}]}`,
			want: true,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
		{
			name: "invalid json",
			body: `{"model":`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequestBodyHasImageInput([]byte(tc.body)); got != tc.want {
				t.Fatalf("RequestBodyHasImageInput() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRewriteImageInputModel(t *testing.T) {
	modelMap := map[string]string{"deepseek-v4-flash": "glm-5.3-flash"}

	t.Run("rewrites when image input present", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"aGVsbG8="}}]}]}`)
		newBody, newModel, changed := RewriteImageInputModel(body, modelMap)
		if !changed {
			t.Fatal("expected rewrite")
		}
		if newModel != "glm-5.3-flash" {
			t.Fatalf("newModel = %q", newModel)
		}
		if !gjson.ValidBytes(newBody) || gjson.GetBytes(newBody, "model").String() != "glm-5.3-flash" {
			t.Fatalf("body model not rewritten: %s", newBody)
		}
	})

	t.Run("no rewrite without image input", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`)
		_, _, changed := RewriteImageInputModel(body, modelMap)
		if changed {
			t.Fatal("should not rewrite pure text request")
		}
	})

	t.Run("no rewrite for unmapped model", func(t *testing.T) {
		body := []byte(`{"model":"other-model","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"aGVsbG8="}}]}]}`)
		_, _, changed := RewriteImageInputModel(body, modelMap)
		if changed {
			t.Fatal("should not rewrite unmapped model")
		}
	})

	t.Run("matches claude code long-context suffix", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash[1m]","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"aGVsbG8="}}]}]}`)
		_, newModel, changed := RewriteImageInputModel(body, modelMap)
		if !changed || newModel != "glm-5.3-flash" {
			t.Fatalf("expected rewrite via [1m] suffix, changed=%v newModel=%q", changed, newModel)
		}
	})

	t.Run("empty map is no-op", func(t *testing.T) {
		body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"aGVsbG8="}}]}]}`)
		_, _, changed := RewriteImageInputModel(body, nil)
		if changed {
			t.Fatal("empty map should not rewrite")
		}
	})
}
