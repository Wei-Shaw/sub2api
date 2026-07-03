package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// kiroAvailableProfile 对应 ListAvailableProfiles API 返回的单个 profile。
type kiroAvailableProfile struct {
	ARN         string `json:"arn"`
	ProfileName string `json:"profileName"`
}

// kiroListAvailableProfilesResponse 对应 ListAvailableProfiles API 的响应。
type kiroListAvailableProfilesResponse struct {
	Profiles  []kiroAvailableProfile `json:"profiles"`
	NextToken string                 `json:"nextToken"`
}

// firstARN 返回第一个非空的真实 profileArn。
func (r *kiroListAvailableProfilesResponse) firstARN() string {
	for _, p := range r.Profiles {
		if arn := strings.TrimSpace(p.ARN); arn != "" {
			return arn
		}
	}
	return ""
}

// kiroProfileResolutionFlight 用于对同一账号的 profileArn 解析做进程内去重，
// 避免并发请求重复调用 ListAvailableProfiles API。
var kiroProfileResolutionFlight sync.Map // map[int64]*sync.Once

// kiroListAvailableProfiles 调用 AWS CodeWhisperer ListAvailableProfiles API 获取真实 profileArn。
//
// API: POST https://q.{region}.amazonaws.com/
// Header: x-amz-target: AmazonCodeWhispererService.ListAvailableProfiles
// Content-Type: application/x-amz-json-1.0
// Body: {"maxResults":10}
func kiroListAvailableProfiles(ctx context.Context, account *Account, token string) (*kiroListAvailableProfilesResponse, error) {
	if account == nil {
		return nil, fmt.Errorf("account is nil")
	}
	region := kiroAPIRegion(account)
	host := fmt.Sprintf("q.%s.amazonaws.com", region)
	endpointURL := fmt.Sprintf("https://%s/", host)

	reqBody := `{"maxResults":10}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create list profiles request: %w", err)
	}

	accountKey := buildKiroAccountKey(account)
	machineID := buildKiroMachineID(account)

	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonCodeWhispererService.ListAvailableProfiles")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", kiropkg.BuildRuntimeUserAgent(accountKey, machineID))
	req.Header.Set("X-Amz-User-Agent", kiropkg.BuildRuntimeAmzUserAgent(accountKey, machineID))
	req.Header.Set("Amz-Sdk-Request", "attempt=1; max=1")
	req.Header.Set("Amz-Sdk-Invocation-Id", uuid.NewString())
	applyKiroConditionalHeaders(req, account)

	proxyURL := kiroProxyURL(account)
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:           proxyURL,
		Timeout:            30 * time.Second,
		ValidateResolvedIP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list available profiles request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list available profiles: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed kiroListAvailableProfilesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &parsed, nil
}

// kiroResolveAndPersistProfileArn 解析并回填 Enterprise/IdC 账号的真实 profileArn（包级共用函数）。
//
// 流式端点（generateAssistantResponse）强制要求 profileArn：不带 → 400
// "profileArn is required for this request"。Enterprise/IdC 账号的 OAuth 流程
// 通常不返回 profileArn，凭据中可能为空或 BuilderID 占位符，需要通过
// ListAvailableProfiles API 获取真实 ARN。
//
// 行为：
//   - API Key 凭据 / 已有真实（非占位符）profileArn → 直接返回，不发起网络请求
//   - 否则调用 ListAvailableProfiles，命中真实 ARN 时写回凭据并持久化到 DB
//   - 上游无 profile（如 Social/BuilderID 账号）→ 回填默认 ARN（Social → Social ARN，其余 → BuilderID 占位符）并持久化
//   - 进程内去重：同一账号仅首次请求时触发 API 调用，避免重复查询
func kiroResolveAndPersistProfileArn(ctx context.Context, repo AccountRepository, account *Account, token string) string {
	if account == nil {
		return ""
	}

	// API Key 凭据没有 profileArn 概念
	authMethod := strings.TrimSpace(account.GetCredential("auth_method"))
	if strings.EqualFold(authMethod, "api_key") || strings.EqualFold(authMethod, "apikey") {
		return ""
	}
	if firstKiroCredential(account, "kiro_api_key", "kiroApiKey", "api_key") != "" {
		return ""
	}

	// 已有真实 ARN（非占位符）→ 直接用
	existingARN := strings.TrimSpace(account.GetCredential("profile_arn"))
	if existingARN != "" && !kiroIsPlaceholderProfileARN(existingARN) {
		return existingARN
	}

	// 进程内去重：同一账号只尝试一次 ListAvailableProfiles
	accountID := account.ID
	onceVal, _ := kiroProfileResolutionFlight.LoadOrStore(accountID, &sync.Once{})
	once, ok := onceVal.(*sync.Once)
	if !ok {
		return existingARN
	}

	var resolvedARN string
	once.Do(func() {
		arn := kiroDefaultProfileARN(account)

		profiles, err := kiroListAvailableProfiles(ctx, account, token)
		if err != nil {
			// API 失败（如 BuilderID 账号不支持 ListAvailableProfiles），fallback 到默认 ARN
			logger.L().Warn("kiro profileArn resolution failed, using default",
				zap.Int64("account_id", accountID),
				zap.String("profile_arn", arn),
				zap.Error(err),
			)
		} else if real := profiles.firstARN(); real != "" {
			arn = real
		} else {
			// 上游无 Enterprise profile（Social/BuilderID 等），使用默认 ARN 回填
			logger.L().Debug("kiro profileArn resolution: no enterprise profile found, using default",
				zap.Int64("account_id", accountID),
				zap.String("profile_arn", arn),
			)
		}

		resolvedARN = arn

		// 回填到 account 内存对象
		if account.Credentials == nil {
			account.Credentials = make(map[string]any)
		}
		account.Credentials["profile_arn"] = arn

		// 持久化到数据库
		if repo != nil {
			if persistErr := persistAccountCredentials(ctx, repo, account, account.Credentials); persistErr != nil {
				logger.L().Warn("kiro profileArn persist failed (does not affect current request)",
					zap.Int64("account_id", accountID),
					zap.Error(persistErr),
				)
			}
		}

		logger.L().Info("kiro profileArn resolved and persisted",
			zap.Int64("account_id", accountID),
			zap.String("profile_arn", arn),
		)
	})

	if resolvedARN != "" {
		return resolvedARN
	}
	return existingARN
}

// resolveAndPersistKiroProfileArn 是 GatewayService 对 kiroResolveAndPersistProfileArn 的 thin wrapper。
func (s *GatewayService) resolveAndPersistKiroProfileArn(ctx context.Context, account *Account, token string) string {
	return kiroResolveAndPersistProfileArn(ctx, s.accountRepo, account, token)
}

// ensureKiroProfileArnForRequest 确保 Kiro 请求的 profileArn 已解析。
// 在流式/非流式请求发送前调用，如果是 KRS 模式且 profileArn 缺失或为占位符，
// 则触发 ListAvailableProfiles 解析并回填。
func (s *GatewayService) ensureKiroProfileArnForRequest(ctx context.Context, account *Account, token string, mode string) {
	if account == nil || mode != KiroEndpointModeKRS {
		return
	}
	existingARN := strings.TrimSpace(account.GetCredential("profile_arn"))
	if existingARN != "" && !kiroIsPlaceholderProfileARN(existingARN) {
		return
	}
	// 触发解析（内部有去重逻辑）
	_ = s.resolveAndPersistKiroProfileArn(ctx, account, token)
}
