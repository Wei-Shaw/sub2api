package service

import "context"

// TokenProvider 定义"为某个账号取得可用 access token"的能力。
//
// 五个平台的 *TokenProvider 早已实现同一套方法，只是没有声明接口：
// Claude / OpenAI / Gemini / Antigravity / Grok 的 GetAccessToken 签名逐字相同。
// 这里只把已经存在的事实写出来，不改变任何行为。
//
// 与 TokenRefresher 一样，接口只包含行为方法。SetRefreshAPI 和 SetRefreshPolicy
// 是 wire 期的接线，调用方（16 处）无一使用，因此不放进接口。
type TokenProvider interface {
	// GetAccessToken 返回该账号当前可用的 access token，必要时先刷新。
	GetAccessToken(ctx context.Context, account *Account) (string, error)
}

// 编译期断言：五个平台的 provider 都满足 TokenProvider。
// 任何一个的 GetAccessToken 签名漂移，都会在这里编译失败，而不是等到
// 有人试图把它们放进同一个容器时才发现。
var (
	_ TokenProvider = (*ClaudeTokenProvider)(nil)
	_ TokenProvider = (*OpenAITokenProvider)(nil)
	_ TokenProvider = (*GeminiTokenProvider)(nil)
	_ TokenProvider = (*AntigravityTokenProvider)(nil)
	_ TokenProvider = (*GrokTokenProvider)(nil)
)
