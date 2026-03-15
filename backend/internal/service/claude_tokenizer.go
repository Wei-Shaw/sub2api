package service

import (
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
	tiktokenloader "github.com/pkoukk/tiktoken-go-loader"
)

var (
	claudeTokenizerOnce sync.Once
	claudeTokenizer     *tiktoken.Tiktoken
)

func getClaudeTokenizer() *tiktoken.Tiktoken {
	claudeTokenizerOnce.Do(func() {
		// Use offline loader to avoid runtime dictionary download.
		tiktoken.SetBpeLoader(tiktokenloader.NewOfflineLoader())
		// Use a high-capacity tokenizer as the default approximation for Claude payloads.
		enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
		if err != nil {
			enc, err = tiktoken.GetEncoding(tiktoken.MODEL_CL100K_BASE)
		}
		if err == nil {
			claudeTokenizer = enc
		}
	})
	return claudeTokenizer
}

func estimateTokensByThirdPartyTokenizer(text string) (int, bool) {
	enc := getClaudeTokenizer()
	if enc == nil {
		return 0, false
	}
	tokens := len(enc.EncodeOrdinary(text))
	if tokens <= 0 {
		return 0, false
	}
	return tokens, true
}
