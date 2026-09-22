package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/gin-gonic/gin"
)

const (
	// adobeTestTimeout 比生产的 adobe.DefaultImageTimeout(180s) 短：管理端的 SSE
	// 不该为一次连通性验证挂三分钟，宁可提前报超时让运维重试。
	adobeTestTimeout = 90 * time.Second

	// adobeTestSize 固定 1K：测试只验证链路，没必要按 4K 的价烧额度。
	// （ResolutionFromSize 按长边定档，1024 → 1K。）
	adobeTestSize = "1024x1024"

	adobeTestDefaultPrompt = defaultOpenAIImageTestPrompt
)

// adobeTestClients 是账号测试专用的 Adobe 客户端缓存。
//
// 刻意不复用 AdobeImageService：那条路会把产物过 ImageResultUploader.Rewrite
// 传到对象存储——一张用完即弃的测试图不值得污染存储桶，也不该多一个外部失败点。
// 测试要的是原始字节，直接塞进 SSE 的 data: URL 给浏览器。
var adobeTestClients = &adobeClientCache{}

// resolveAdobeTestPrompt 与 resolveGrokImagePrompt 同形：空则用其它生图渠道同一句默认。
func resolveAdobeTestPrompt(prompt string) string {
	if trimmed := strings.TrimSpace(prompt); trimmed != "" {
		return trimmed
	}
	return adobeTestDefaultPrompt
}

// adobeTestAccessToken 取本次测试要用的 access_token。
//
// access_token 在账号表单上是**可选**的——文案明说「留空则首次刷新时用 cookie 自动换取」。
// 所以刚建好的账号必然没有 token（后台刷新器还没轮到它），此时直接报错等于让运维
// 干等一个看不见的定时任务。这里改成当场用 cookie 换一个，顺带把「cookie 还能不能
// 换到 token」也测了——那本来就是这条链路最该验证的第一步。
//
// 换到的 token 会落库：下一次测试与生产请求都能直接复用，与后台刷新器写的是同一个字段。
func (s *AccountTestService) adobeTestAccessToken(ctx context.Context, c *gin.Context, account *Account) (string, error) {
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token != "" && !adobe.IsTokenExpired(token, adobeRefreshWindow) {
		return token, nil
	}

	if strings.TrimSpace(account.GetCredential("cookie")) == "" {
		if token == "" {
			return "", errors.New("adobe account has neither an access_token nor a cookie — paste the cookie exported from a logged-in Firefly page")
		}
		// 有 token 但已过期且无 cookie 可换：照用，让上游给出权威的 401。
		return token, nil
	}

	reason := "access_token is missing"
	if token != "" {
		reason = "access_token has expired"
	}
	s.sendEvent(c, TestEvent{Type: "status", Text: fmt.Sprintf("%s — exchanging a fresh one from the cookie...", reason)})

	// 与网关共用 AdobeTokenProvider：进程内锁 + Redis 锁 + DB 重读，落库只按 cookie 条件合并
	// token 字段，不会把管理员刚改的 cookie / model_mapping 覆盖回旧值。
	refreshed, err := s.adobeTestTokenProvider().GetAccessToken(ctx, account)
	if err != nil {
		return "", errors.New(formatAdobeTestError(err))
	}
	s.sendEvent(c, TestEvent{Type: "status", Text: "Fresh access_token obtained from cookie."})
	return refreshed, nil
}

// adobeTestTokenProvider 返回注入的共享 provider；未注入时（单测等）用测试 client 缓存构造
// 一个仅进程内加锁的 provider，落库语义保持一致。
func (s *AccountTestService) adobeTestTokenProvider() *AdobeTokenProvider {
	if s.adobeTokenProvider != nil {
		return s.adobeTokenProvider
	}
	return newAdobeTokenProvider(s.accountRepo, nil, &AdobeTokenRefresher{clients: adobeTestClients})
}

// testAdobeAccountConnection 对 Adobe 账号做一次真实出图，验证完整链路：
// token → 身份头 → payload 形状 → submit → poll → download。
//
// 改前 Adobe 账号会落到 testClaudeAccountConnection，拿 Adobe 的 IMS token 去打
// api.anthropic.com，必然 401。
func (s *AccountTestService) testAdobeAccountConnection(
	c *gin.Context, account *Account, modelID, prompt string,
) error {
	if isAdobeRelayAccount(account) {
		testModel := strings.TrimSpace(modelID)
		if testModel == "" {
			if ids := adobe.ImageModelIDs(); len(ids) > 0 {
				testModel = ids[0]
			}
		}
		testModel = account.GetMappedModel(testModel)
		return s.testOpenAIImageAPIKey(c, c.Request.Context(), account, testModel, resolveAdobeTestPrompt(prompt))
	}

	ctx := c.Request.Context()

	token, err := s.adobeTestAccessToken(ctx, c, account)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	requestedModel := strings.TrimSpace(modelID)
	if requestedModel == "" {
		if ids := adobe.ImageModelIDs(); len(ids) > 0 {
			requestedModel = ids[0]
		}
	}
	if requestedModel == "" {
		return s.sendErrorAndEnd(c, "No Adobe image model available to test")
	}

	upstreamModelID := account.GetMappedModel(requestedModel)
	conf, resolveErr := adobe.ResolveImage(adobe.ImageRequest{ModelID: upstreamModelID, Size: adobeTestSize})
	if resolveErr != nil {
		return s.sendErrorAndEnd(c, formatAdobeTestError(resolveErr))
	}

	s.prepareGrokTestSSE(c)
	s.sendEvent(c, TestEvent{Type: "test_start", Model: requestedModel})

	// 把映射结果暴露出来：运维能直接看到「我选的 gpt-image-2.5-flare 实际打到了哪个
	// upstream modelVersion」——这正是排查静默降级的手段。
	s.sendEvent(c, TestEvent{Type: "status", Text: fmt.Sprintf(
		"Resolved %s → %s (upstream modelId=%s modelVersion=%s, %s %s)",
		requestedModel, conf.Family, conf.UpstreamModelID, conf.UpstreamModelVersion,
		conf.OutputResolution, conf.AspectRatio)})
	s.sendEvent(c, TestEvent{Type: "status", Text: "Submitting generation job to Adobe Firefly..."})

	client := adobeTestClients.clientForAccount(account)
	generated, err := client.GenerateImage(ctx, adobe.GenerateImageInput{
		Token:        token,
		Timeout:      adobeTestTimeout,
		ARPSessionID: account.GetCredential("arp_session_id"),
		Options: adobe.ImagePayloadOptions{
			Prompt:               resolveAdobeTestPrompt(prompt),
			AspectRatio:          conf.AspectRatio,
			OutputResolution:     conf.OutputResolution,
			UpstreamModelID:      conf.UpstreamModelID,
			UpstreamModelVersion: conf.UpstreamModelVersion,
			PayloadKind:          conf.PayloadKind,
			SizePixels:           conf.SizePixels,
		},
	})
	if err != nil {
		return s.sendErrorAndEnd(c, formatAdobeTestError(err))
	}
	if generated == nil || len(generated.Bytes) == 0 {
		return s.sendErrorAndEnd(c, "Adobe returned an empty image")
	}

	s.sendEvent(c, TestEvent{
		Type:     "image",
		ImageURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(generated.Bytes),
		MimeType: "image/png",
	})
	s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf(
		"generated %d bytes via %s\n", len(generated.Bytes), conf.ModelID)})
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// formatAdobeTestError 把 pkg 的类型化错误翻译成运维看得懂的处置建议。
// 用 errors.As 而不是类型断言：pkg 层可能用 fmt.Errorf 包过一层。
func formatAdobeTestError(err error) string {
	if err == nil {
		return "unknown Adobe error"
	}
	var (
		authErr     *adobe.AuthError
		quotaErr    *adobe.QuotaExhaustedError
		entitledErr *adobe.NotEntitledError
		tempErr     *adobe.UpstreamTemporaryError
		rejectedErr *adobe.ContentRejectedError
	)
	switch {
	case errors.As(err, &entitledErr):
		return fmt.Sprintf("Adobe account is not entitled to this model or quality tier (%d) — a higher-plan Firefly account may succeed: %s",
			entitledErr.StatusCode, err.Error())
	case errors.As(err, &authErr):
		return fmt.Sprintf("Adobe credentials rejected (%d) — the cookie has likely expired, re-export it from a logged-in Firefly page: %s",
			authErr.StatusCode, err.Error())
	case errors.As(err, &quotaErr):
		return fmt.Sprintf("Adobe credits exhausted — free accounts get 10 generations per day and reset at 00:00 UTC: %s", err.Error())
	case errors.As(err, &rejectedErr):
		return fmt.Sprintf("Adobe rejected the image content: %s", rejectedErr.User())
	case errors.As(err, &tempErr):
		return fmt.Sprintf("Adobe upstream temporarily unavailable, retry later: %s", err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("Adobe generation timed out after %s: %s", adobeTestTimeout, err.Error())
	default:
		return err.Error()
	}
}
