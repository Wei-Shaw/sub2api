// Package headscript renders administrator-controlled scripts for the frontend head.
package headscript

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

const MaxBytes = 64 * 1024

// Render accepts JavaScript or a fragment containing only script tags and comments.
// This is trusted administrator code, not sanitized user content.
func Render(source, nonce string) (string, error) {
	if len(source) > MaxBytes {
		return "", fmt.Errorf("custom head script must not exceed 64 KiB")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}
	if !strings.HasPrefix(source, "<") {
		if strings.Contains(strings.ToLower(source), "</script") {
			return "", fmt.Errorf("escape closing script tags inside JavaScript strings")
		}
		return scriptStart(nil, nonce) + source + "</script>", nil
	}

	z := html.NewTokenizer(strings.NewReader(source))
	var result strings.Builder
	inScript := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				return "", z.Err()
			}
			if len(z.Raw()) != 0 {
				return "", fmt.Errorf("custom head script contains an incomplete HTML tag")
			}
			if inScript {
				return "", fmt.Errorf("custom head script contains an unclosed script tag")
			}
			return result.String(), nil
		case html.StartTagToken:
			token := z.Token()
			if inScript || token.Data != "script" {
				return "", fmt.Errorf("custom head script may only contain script tags")
			}
			_, _ = result.WriteString(scriptStart(token.Attr, nonce))
			inScript = true
		case html.EndTagToken:
			token := z.Token()
			if !inScript || token.Data != "script" {
				return "", fmt.Errorf("custom head script contains an unexpected closing tag")
			}
			_, _ = result.WriteString("</script>")
			inScript = false
		case html.TextToken:
			raw := string(z.Raw())
			if !inScript && strings.TrimSpace(raw) != "" {
				return "", fmt.Errorf("place JavaScript inside script tags when using HTML")
			}
			_, _ = result.WriteString(raw)
		case html.CommentToken:
			_, _ = result.Write(z.Raw())
		default:
			return "", fmt.Errorf("custom head script may only contain script tags")
		}
	}
}

func scriptStart(attrs []html.Attribute, nonce string) string {
	token := html.Token{Type: html.StartTagToken, Data: "script"}
	for _, attr := range attrs {
		if attr.Key != "nonce" && attr.Key != "data-custom-head-script" {
			token.Attr = append(token.Attr, attr)
		}
	}
	token.Attr = append(token.Attr,
		html.Attribute{Key: "nonce", Val: nonce},
		html.Attribute{Key: "data-custom-head-script", Val: ""},
	)
	return token.String()
}
