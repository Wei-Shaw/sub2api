package signature

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ReplaceThinkingSignaturesInBody rewrites every `signature` field inside
// messages[].content[] blocks of type "thinking" with a value drawn from pool.
// Pool entries are consumed in order (pool[i] for the i-th encountered thinking
// signature); when i >= len(pool), replacement cycles back to pool[i % len(pool)].
//
// Returns the modified body and the count of signatures that were actually
// replaced. If pool is empty or the body contains no thinking signature,
// returns (body, 0) unchanged.
func ReplaceThinkingSignaturesInBody(body []byte, pool []string) ([]byte, int) {
	if len(pool) == 0 {
		return body, 0
	}
	// Fast path: no signature field at all.
	if !bytes.Contains(body, []byte(`"signature"`)) {
		return body, 0
	}

	root := gjson.ParseBytes(body)
	msgs := root.Get("messages")
	if !msgs.Exists() || !msgs.IsArray() {
		return body, 0
	}

	out := body
	replaced := 0
	mIdx := 0
	msgs.ForEach(func(_, msg gjson.Result) bool {
		thisMsg := mIdx
		mIdx++
		content := msg.Get("content")
		if !content.IsArray() {
			return true
		}
		bIdx := 0
		content.ForEach(func(_, blk gjson.Result) bool {
			thisBlk := bIdx
			bIdx++
			if blk.Get("type").String() != "thinking" {
				return true
			}
			sig := blk.Get("signature")
			if !sig.Exists() || sig.String() == "" {
				return true
			}
			newSig := pool[replaced%len(pool)]
			path := fmt.Sprintf("messages.%d.content.%d.signature", thisMsg, thisBlk)
			if next, err := sjson.SetBytes(out, path, newSig); err == nil {
				out = next
				replaced++
			}
			return true
		})
		return true
	})

	return out, replaced
}

// ReplaceThinkingSignaturesInClaudeRequest rewrites signatures in every
// thinking block of the ClaudeRequest's Messages. Semantics mirror
// ReplaceThinkingSignaturesInBody but operate on the struct form used by the
// Antigravity path before it re-transforms to Gemini.
//
// Returns the number of signatures replaced. An error is returned only when
// a message's content cannot be re-marshaled after mutation.
func ReplaceThinkingSignaturesInClaudeRequest(req *antigravity.ClaudeRequest, pool []string) (int, error) {
	if req == nil || len(pool) == 0 {
		return 0, nil
	}

	replaced := 0
	for i := range req.Messages {
		raw := req.Messages[i].Content
		if len(raw) == 0 {
			continue
		}

		// String content has no blocks to touch.
		var str string
		if json.Unmarshal(raw, &str) == nil {
			continue
		}

		var blocks []map[string]any
		if err := json.Unmarshal(raw, &blocks); err != nil {
			continue
		}

		modified := false
		for _, block := range blocks {
			t, _ := block["type"].(string)
			if t != "thinking" {
				continue
			}
			sig, ok := block["signature"].(string)
			if !ok || sig == "" {
				continue
			}
			block["signature"] = pool[replaced%len(pool)]
			replaced++
			modified = true
		}

		if !modified {
			continue
		}

		newRaw, err := json.Marshal(blocks)
		if err != nil {
			return replaced, err
		}
		req.Messages[i].Content = newRaw
	}

	return replaced, nil
}
