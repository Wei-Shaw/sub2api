package headscript

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
	}{
		{"empty", " \n ", ""},
		{"javascript", `window.example = 1 < 2 && true;`, `<script nonce="test-nonce" data-custom-head-script="">window.example = 1 < 2 && true;</script>`},
		{"external", `<SCRIPT async src="https://example.com/a.js?a=1&amp;b=2" nonce="old"></SCRIPT>`, `<script async="" src="https://example.com/a.js?a=1&amp;b=2" nonce="test-nonce" data-custom-head-script=""></script>`},
		{"inline", `<script>window.example = "<head>"; if (1 < 2 && true) {}</script>`, `<script nonce="test-nonce" data-custom-head-script="">window.example = "<head>"; if (1 < 2 && true) {}</script>`},
		{"multiple", `<!-- analytics --><script src="/a.js"></script><script type="module">import "/b.js";</script>`, `<!-- analytics --><script src="/a.js" nonce="test-nonce" data-custom-head-script=""></script><script type="module" nonce="test-nonce" data-custom-head-script="">import "/b.js";</script>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.source, "test-nonce")
			if err != nil || got != tc.want {
				t.Fatalf("Render() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestRenderRejectsInvalidFragments(t *testing.T) {
	for _, source := range []string{
		`<script`,
		`<script src="unterminated`,
		`<script>ok()</script><script`,
		`<script>missing close`,
		`<script />`,
		`<script>ok()</script></head><body>`,
		`<img src="x" onerror="alert(1)">`,
		`<script>ok()</script>unwrapped()`,
		`const value = "</ScRiPt><body>";`,
		strings.Repeat("a", MaxBytes+1),
	} {
		if rendered, err := Render(source, "test"); err == nil || rendered != "" {
			t.Errorf("Render(%q) must reject invalid source", source[:min(len(source), 100)])
		}
	}
}
