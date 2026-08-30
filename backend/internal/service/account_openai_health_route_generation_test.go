package service

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestAccountOpenAIHealthRouteGenerationIsPrivateAndFailClosed(t *testing.T) {
	for _, value := range []*int64{nil, int64Pointer(0), int64Pointer(-1)} {
		account := &Account{}
		account.SetOpenAIHealthRouteGenerationFromDurable(value)
		if got, ok := account.OpenAIHealthRouteGeneration(); ok || got != 0 {
			t.Fatalf("invalid durable value %v = (%d, %v), want (0, false)", value, got, ok)
		}
	}

	value := int64(17)
	account := &Account{}
	account.SetOpenAIHealthRouteGenerationFromDurable(&value)
	if got, ok := account.OpenAIHealthRouteGeneration(); !ok || got != value {
		t.Fatalf("durable value = (%d, %v), want (%d, true)", got, ok, value)
	}

	payload, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("marshal account: %v", err)
	}
	if bytes.Contains(payload, []byte("openai_health_route_generation")) {
		t.Fatalf("private durable carrier appeared in JSON: %s", payload)
	}
	var decoded Account
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal account cache payload: %v", err)
	}
	if _, ok := decoded.OpenAIHealthRouteGeneration(); ok {
		t.Fatal("cache-deserialized account retained private durable carrier")
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
