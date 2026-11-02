package openai_decisions

import (
	"strings"
	"testing"
)

const validRequest = `{"model":"gpt-6-luna","input":"The order arrived damaged.","questions":[{"type":"predicate","name":"damaged","instructions":"Is the order damaged?"},{"type":"choice","name":"route","instructions":"Choose a team.","choices":[{"value":"billing"},{"value":"support"}]},{"type":"score","name":"severity","instructions":"Score severity.","levels":[{"label":"low"},{"label":"high"}]}]}`

func TestParseRequestValidatesOpenAIShape(t *testing.T) {
	req, err := ParseRequest([]byte(validRequest))
	if err != nil || req.Model != Model || len(req.Questions) != 3 {
		t.Fatalf("ParseRequest() = %#v, %v", req, err)
	}

	imageRequest := `{"model":"gpt-6-luna","input":[{"role":"user","content":[{"type":"input_text","text":"red"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8=","detail":"high"}]}],"questions":[{"type":"predicate","instructions":"Is it red?"}]}`
	parsed, err := ParseRequest([]byte(imageRequest))
	if err != nil || parsed.ImageCount != 1 {
		t.Fatalf("image request = %#v, %v", parsed, err)
	}
}

func TestParseRequestRejectsUnsupportedInputAndQuestions(t *testing.T) {
	cases := []string{
		`{"model":"gpt-6-luna","input":"x","stream":true,"questions":[{"type":"predicate","instructions":"x"}]}`,
		`{"model":"gpt-6-luna","input":[{"role":"assistant","content":"x"}],"questions":[{"type":"predicate","instructions":"x"}]}`,
		`{"model":"gpt-6-luna","input":"x","questions":[{"type":"choice","instructions":"x","choices":[{"value":1}]}]}`,
		`{"model":"gpt-6-luna","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.com/a.png"}]}],"questions":[{"type":"predicate","instructions":"x"}]}`,
	}
	for _, body := range cases {
		if _, err := ParseRequest([]byte(body)); err == nil {
			t.Fatalf("accepted invalid request: %s", body)
		}
	}
}

func TestDecodeResponseValidatesOrderedTypedAnswers(t *testing.T) {
	body := `{"model":"gpt-6-luna","answers":[{"type":"predicate","name":"damaged","probability":0.9},{"type":"choice","name":"route","choice":"billing","confidence":0.8,"probabilities":[{"value":"billing","probability":0.8},{"value":"support","probability":0.2}]},{"type":"score","name":"severity","score":0.25,"confidence":0.7,"probabilities":[{"value":0,"label":"low","probability":0.75},{"value":1,"label":"high","probability":0.25}]}],"usage":{"input_tokens":12,"output_tokens":0,"input_tokens_details":{"cached_tokens":3}}}`
	usage, err := DecodeResponse([]byte(validRequest), []byte(body))
	if err != nil || usage.InputTokens != 12 || usage.CachedTokens != 3 {
		t.Fatalf("DecodeResponse() = %#v, %v", usage, err)
	}
	for _, invalid := range []string{
		strings.Replace(body, `"probability":0.9`, `"probability":1.2`, 1),
		strings.Replace(body, `"probability":0.8},{"value":"support","probability":0.2`, `"probability":0.1},{"value":"support","probability":0.2`, 1),
		strings.Replace(body, `"choice":"billing"`, `"choice":"unknown"`, 1),
		strings.Replace(body, `"model":"gpt-6-luna"`, `"model":"other"`, 1),
	} {
		if _, err := DecodeResponse([]byte(validRequest), []byte(invalid)); err == nil {
			t.Fatalf("accepted invalid response: %s", invalid)
		}
	}
}
