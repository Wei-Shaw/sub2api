// Package anthropictokenizer provides a local port of Anthropic's
// @anthropic-ai/tokenizer package for rough Claude token estimation.
package anthropictokenizer

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
	"golang.org/x/text/unicode/norm"
)

// claudeJSON is copied from Anthropic's anthropic-tokenizer-typescript package.
//
//go:embed claude.json
var claudeJSON []byte

type claudeEncoding struct {
	ExplicitNVocab int            `json:"explicit_n_vocab"`
	PatStr         string         `json:"pat_str"`
	SpecialTokens  map[string]int `json:"special_tokens"`
	BPERanks       string         `json:"bpe_ranks"`
}

var (
	tokenizerOnce sync.Once
	tokenizer     *tiktoken.Tiktoken
	tokenizerErr  error
)

// CountTokens returns the Anthropic reference tokenizer count for text.
// It follows @anthropic-ai/tokenizer: NFKC normalization and all special
// tokens allowed.
func CountTokens(text string) int {
	if text == "" {
		return 0
	}
	tok, err := getTokenizer()
	if err != nil {
		return fallbackTokenCount(text)
	}
	return len(tok.Encode(norm.NFKC.String(text), []string{"all"}, nil))
}

func getTokenizer() (*tiktoken.Tiktoken, error) {
	tokenizerOnce.Do(func() {
		tokenizer, tokenizerErr = newTokenizer()
	})
	return tokenizer, tokenizerErr
}

func newTokenizer() (*tiktoken.Tiktoken, error) {
	var enc claudeEncoding
	if err := json.Unmarshal(claudeJSON, &enc); err != nil {
		return nil, fmt.Errorf("parse claude tokenizer: %w", err)
	}
	ranks, err := parseBPERanks(enc.BPERanks, len(enc.SpecialTokens))
	if err != nil {
		return nil, err
	}
	core, err := tiktoken.NewCoreBPE(ranks, enc.SpecialTokens, enc.PatStr)
	if err != nil {
		return nil, err
	}
	specialSet := make(map[string]any, len(enc.SpecialTokens))
	for token := range enc.SpecialTokens {
		specialSet[token] = true
	}
	return tiktoken.NewTiktoken(core, &tiktoken.Encoding{
		Name:           "claude",
		PatStr:         enc.PatStr,
		MergeableRanks: ranks,
		SpecialTokens:  enc.SpecialTokens,
		ExplicitNVocab: enc.ExplicitNVocab,
	}, specialSet), nil
}

func parseBPERanks(raw string, rankOffset int) (map[string]int, error) {
	parts := strings.Fields(raw)
	ranks := make(map[string]int, len(parts))
	for i, part := range parts {
		token, err := base64.StdEncoding.DecodeString(part)
		if err != nil {
			return nil, fmt.Errorf("decode claude bpe rank %d: %w", i, err)
		}
		ranks[string(token)] = i + rankOffset
	}
	if len(ranks) != len(parts) {
		return nil, fmt.Errorf("claude tokenizer has duplicate bpe ranks")
	}
	return ranks, nil
}

func fallbackTokenCount(text string) int {
	count := len([]rune(strings.TrimSpace(text))) / 4
	if count == 0 {
		return 1
	}
	return count
}
