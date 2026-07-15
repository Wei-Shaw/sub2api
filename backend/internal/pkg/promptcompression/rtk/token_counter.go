package rtk

import (
	"fmt"
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// TiktokenCounter uses the encodings already vendored by sub2api. Unknown and
// Anthropic models are checked against both common encodings by the engine.
type TiktokenCounter struct{}

func (TiktokenCounter) Count(model, text string) (int, error) {
	codec, err := codecForModel(model)
	if err != nil {
		return 0, err
	}
	return codec.Count(text)
}

func (TiktokenCounter) ConservativeCount(model, text string) (int, int, error) {
	left, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return 0, 0, err
	}
	right, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return 0, 0, err
	}
	a, err := left.Count(text)
	if err != nil {
		return 0, 0, err
	}
	b, err := right.Count(text)
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

func codecForModel(model string) (tokenizer.Codec, error) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return nil, fmt.Errorf("tokenizer model is empty")
	}
	// The tokenizer package exposes model aliases, but using explicit encodings
	// keeps unknown provider model names fail-open and deterministic.
	encoding := tokenizer.Cl100kBase
	if strings.Contains(m, "gpt-4o") || strings.Contains(m, "o1") || strings.Contains(m, "o3") ||
		strings.Contains(m, "o4") || strings.Contains(m, "gpt-5") {
		encoding = tokenizer.O200kBase
	}
	return tokenizer.Get(encoding)
}
