package tutorialhtml

import (
	"strings"
	"testing"
)

func TestSanitizeTutorialHTML_RemovesDangerousContentAndKeepsSafeChineseContent(t *testing.T) {
	raw := `
<script>alert(1)</script>
<style>body{display:none}</style>
<iframe src="https://evil.example"></iframe>
<img src="x" onerror="alert(1)" alt="中文图片说明">
<a href="javascript:alert(1)" onclick="alert(1)">危险链接</a>
<a href="https://example.com">查看完整中文接入说明</a>
<div style="position:fixed;top:0;left:0;width:100%;height:100%">overlay</div>
<div style="color:#0f766e;text-align:center">中文标题测试</div>
<pre><code class="language-javascript">const ok = true;</code></pre>
<blockquote>“这是一个中文引用块。”</blockquote>
`

	got := SanitizeTutorialHTML(raw)

	for _, forbidden := range []string{
		"<script",
		"<style",
		"<iframe",
		"onerror=",
		"onclick=",
		"javascript:alert(1)",
		"position:fixed",
		"width:100%",
		"height:100%",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized html should not contain %q, got: %s", forbidden, got)
		}
	}

	for _, expected := range []string{
		"查看完整中文接入说明",
		"中文标题测试",
		"language-javascript",
		"这是一个中文引用块",
		`rel="noopener noreferrer nofollow"`,
		`style="color: #0f766e; text-align: center"`,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("sanitized html should contain %q, got: %s", expected, got)
		}
	}
}

func TestSanitizeTutorialHTML_RoundTripStable(t *testing.T) {
	raw := `
<h1>sub2api 接入教程：中文显示测试</h1>
<p>这是一个用于验证中文显示的教程页面。</p>
<p><script>alert(1)</script></p>
<p><a href="javascript:alert(1)">危险链接</a></p>
<p><a href="https://example.com">查看完整中文接入说明</a></p>
<blockquote>“这是一个中文引用块。”</blockquote>
<pre><code class="language-javascript">console.log('safe code')</code></pre>
<p><img src="/api/v1/pages/tutorial/images/demo.png" alt="中文图片说明" onerror="alert(1)"></p>
`

	first := SanitizeTutorialHTML(raw)
	second := SanitizeTutorialHTML(first)

	if first != second {
		t.Fatalf("sanitization should be stable across round trips\nfirst: %s\nsecond: %s", first, second)
	}
}

func TestSanitizeTutorialHTML_DropsEmptyUnsafeWrappers(t *testing.T) {
	raw := `
<p><script>alert(1)</script></p>
<p><a href="javascript:alert(1)">危险链接</a></p>
<p><img src="x" alt="坏图片"></p>
`

	got := SanitizeTutorialHTML(raw)

	if strings.Contains(got, "<p></p>") {
		t.Fatalf("sanitized html should not keep empty paragraphs: %s", got)
	}
	if strings.Contains(got, "<a>危险链接</a>") {
		t.Fatalf("sanitized html should unwrap dangerous links instead of keeping empty anchors: %s", got)
	}
	if strings.Contains(got, `<img alt="坏图片">`) {
		t.Fatalf("sanitized html should drop images without safe src: %s", got)
	}
	if !strings.Contains(got, "危险链接") {
		t.Fatalf("sanitized html should preserve dangerous link text as plain text: %s", got)
	}
}
