package openai_decisions

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const mixedRequest = `{"model":"gpt-6-luna","input":"synthetic evidence","questions":[{"type":"predicate","name":"duplicate","instructions":"Is it synthetic?"},{"type":"choice","name":"duplicate","instructions":"Select a value.","choices":[{"value":true},{"value":"true"},{"value":"c0"}]},{"type":"score","instructions":"Rate urgency.","levels":[{"label":"Low"},{"label":"High"}]}]}`
const mixedResponse = `{"model":"openai/gpt-6-luna-decisions","answers":{"q2":{"type":"score","score":0.25,"confidence":0.9,"probabilities":{"1":0.25,"0":0.75}},"q0":{"type":"noul","noul":0.95},"q1":{"type":"choice","choice":"c0","confidence":0.8,"probabilities":{"c2":0.1,"c0":0.7,"c1":0.2}}},"usage":{"input_tokens":100,"output_tokens":0,"cost":0.00001}}`

func TestBridgePreservesOrderedNamesAndTypedChoices(t *testing.T) {
	encoded, err := EncodeOpenRouterRequest([]byte(mixedRequest))
	if err != nil {
		t.Fatal(err)
	}
	// Decode just the choice question since score criteria is an array.
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &payload)
	var questions map[string]json.RawMessage
	_ = json.Unmarshal(payload["questions"], &questions)
	var choice struct {
		Criteria map[string]string `json:"criteria"`
	}
	_ = json.Unmarshal(questions["q1"], &choice)
	if choice.Criteria["c0"] != "true" || choice.Criteria["c1"] != `"true"` || choice.Criteria["c2"] != `"c0"` {
		t.Fatalf("typed options collapsed: %s", encoded)
	}
	decoded, usage, err := DecodeOpenRouterResponse([]byte(mixedRequest), []byte(mixedResponse))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Model   string           `json:"model"`
		Answers []map[string]any `json:"answers"`
	}
	_ = json.Unmarshal(decoded, &result)
	if result.Model != Model || len(result.Answers) != 3 || result.Answers[0]["name"] != "duplicate" || result.Answers[1]["name"] != "duplicate" || result.Answers[2]["name"] != nil || result.Answers[1]["choice"] != true || *usage.InputTokens != 100 {
		t.Fatalf("invalid bridge: %s", decoded)
	}
	probs := result.Answers[1]["probabilities"].([]any)
	if probs[1].(map[string]any)["value"] != "true" {
		t.Fatalf("boolean/string identity lost: %s", decoded)
	}
	levels := result.Answers[2]["probabilities"].([]any)
	if levels[0].(map[string]any)["label"] != "Low" {
		t.Fatalf("labels lost: %s", decoded)
	}
}

func TestBridgeRejectsMissingAndMalformedAnswers(t *testing.T) {
	cases := map[string]string{
		"wrong model":        strings.Replace(mixedResponse, OpenRouterModel, "openai/gpt-6-luna", 1),
		"missing answer":     strings.Replace(mixedResponse, `"q0":{"type":"noul","noul":0.95},`, "", 1),
		"missing confidence": strings.Replace(mixedResponse, `"confidence":0.8,`, "", 1),
		"bad probability":    strings.Replace(mixedResponse, `"noul":0.95`, `"noul":1.1`, 1),
		"bad sum":            strings.Replace(mixedResponse, `"c0":0.7`, `"c0":0.1`, 1),
		"null probability":   strings.Replace(mixedResponse, `"c0":0.7`, `"c0":null`, 1),
		"bad score":          strings.Replace(mixedResponse, `"score":0.25`, `"score":0.8`, 1),
		"bad cost":           strings.Replace(mixedResponse, `"cost":0.00001`, `"cost":-1`, 1),
		"missing usage":      strings.Replace(mixedResponse, `"input_tokens":100,`, "", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := DecodeOpenRouterResponse([]byte(mixedRequest), []byte(body)); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}

func TestBridgePreservesExplicitRefusalAndZeroCost(t *testing.T) {
	body := strings.Replace(mixedResponse, `{"type":"noul","noul":0.95}`, `{"type":"refusal"}`, 1)
	body = strings.Replace(body, `"cost":0.00001`, `"cost":0`, 1)
	out, u, err := DecodeOpenRouterResponse([]byte(mixedRequest), []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"refusal"`) || u.Cost == nil || *u.Cost != 0 {
		t.Fatalf("refusal or zero cost lost: %s", out)
	}
}

func TestOpenRouterSingleImagePreservesContentPartsAndRejectsMultiple(t *testing.T) {
	input := `{"role":"user","content":[{"type":"input_text","text":"red"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}`
	body := []byte(fmt.Sprintf(`{"model":"gpt-6-luna","input":[%s],"questions":[{"type":"predicate","instructions":"Is it red?"}]}`, input))
	wire, err := EncodeOpenRouterRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		State []map[string]any `json:"state"`
	}
	if err = json.Unmarshal(wire, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.State) != 2 || state.State[1]["type"] != "image_url" {
		t.Fatalf("lost image evidence: %s", wire)
	}
	multi := []byte(fmt.Sprintf(`{"model":"gpt-6-luna","input":[%s,%s],"questions":[{"type":"predicate","instructions":"Test?"}]}`, input, input))
	if _, err = ParseRequest(multi); err != nil {
		t.Fatal(err)
	}
	if _, err = EncodeOpenRouterRequest(multi); err == nil {
		t.Fatal("accepted multi-image OpenRouter input")
	}
}
