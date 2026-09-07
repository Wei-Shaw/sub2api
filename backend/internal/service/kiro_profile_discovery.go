package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// kiroProfileDiscoveryRegions 是探测 ListAvailableProfiles 时尝试的固定
// 候选区域列表——照抄参考实现 Kiro-Go 的 defaultKiroProfileRegions
// （proxy/kiro_api.go），刻意不用账号自己的 auth/OIDC 区域现拼一个任意
// host：那是认证区域，不一定托管 Q Developer profile，放开成"任意字符串
// 拼 host"本身也是一个真实的出站请求走私面，不值得为了多探测一个区域
// 冒这个险。
var kiroProfileDiscoveryRegions = []string{"us-east-1", "eu-central-1"}

// kiroListProfilesMaxPages 防止一个失控的分页响应无限循环——参考实现
// Kiro-Go 用 20 页防御，这里保守取 5：真实场景一个账号不会有那么多
// profile 可选，5 页（最多 250 条）已经远超合理范围。
const kiroListProfilesMaxPages = 5

// DiscoverProfileArn 尝试通过 ListAvailableProfiles 自动发现账号的
// profileArn。
//
// 背景：IdC/Builder ID 的 OIDC token 交换本身不会带回 profileArn——真实
// 账号测试证实这一点（我们自己的账号交换后这个字段是空的），起初据此
// 认为只能靠管理员手填。后来查证参考实现 Kiro-Go（proxy/kiro_api.go 的
// ResolveProfileArn/resolveProfileArnAcrossRegions）发现这个结论不完整：
// 存在一个真实可调用的发现接口 ListAvailableProfiles，只是不在 token
// 交换的响应里，需要额外单独调用。
//
// 只探测 kiroProfileDiscoveryRegions 这两个已知区域，命中第一个非空结果
// 就返回。探测本身失败（网络错误、账号在这个区域没有可用 profile、
// Builder ID 账号被 AWS 明确拒绝这个操作——真实错误是 403 "AWS Builder ID
// is not supported for this operation"）都不是硬错误：调用方应该把返回的
// error 当成"这次没发现"处理，不能因为发现失败就阻断账号创建/token 刷新
// 这类主流程——这只是一次锦上添花的尝试，手填入口（KiroCredentialFields.vue
// 的 Profile ARN 字段）仍然保留作为兜底。
func (s *KiroOAuthService) DiscoverProfileArn(ctx context.Context, accessToken, machineID string, proxyID *int64) (string, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", fmt.Errorf("kiro: access token is required to discover profile arn")
	}

	hc, err := s.httpClient(ctx, proxyID)
	if err != nil {
		return "", err
	}

	var lastErr error
	for _, region := range kiroProfileDiscoveryRegions {
		arn, err := s.kiroListFirstProfile(ctx, hc, region, accessToken, machineID)
		if err != nil {
			lastErr = err
			continue
		}
		if arn != "" {
			return arn, nil
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

// kiroListFirstProfile 在单个区域内翻页查询，返回第一条格式合法的 profile
// ARN；该区域没有可用 profile（分页耗尽仍为空）返回 ("", nil)，与"这个
// 区域探测失败"（非 nil error）是两种不同结果，调用方需要分别处理
// （前者应该换下一个候选区域，后者也应该换下一个候选区域但要保留错误
// 供最终兜底汇报）。
func (s *KiroOAuthService) kiroListFirstProfile(ctx context.Context, hc *http.Client, region, accessToken, machineID string) (string, error) {
	url := s.listProfilesHost(region) + "/ListAvailableProfiles"
	nextToken := ""

	for page := 0; page < kiroListProfilesMaxPages; page++ {
		body, err := kiro.BuildListProfilesRequestBody(nextToken)
		if err != nil {
			return "", fmt.Errorf("kiro: build list profiles request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("kiro: build list profiles http request: %w", err)
		}
		req.Header = kiro.BuildHeaders(kiro.HeaderOptions{
			Endpoint:    kiro.Endpoint{Origin: "AI_EDITOR"},
			BearerToken: accessToken,
			MachineID:   machineID,
			Profile:     kiro.DefaultClientProfile(),
		})

		resp, err := hc.Do(req)
		if err != nil {
			return "", fmt.Errorf("kiro: list profiles in %s: %w", region, err)
		}
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, kiroErrorBodyLimit))
		_ = resp.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("kiro: read list profiles response in %s: %w", region, readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("kiro: list profiles in %s returned %d: %s", region, resp.StatusCode, string(respBody))
		}

		parsed, err := kiro.ParseListProfilesResponse(respBody)
		if err != nil {
			return "", err
		}
		if len(parsed.Profiles) > 0 {
			return parsed.Profiles[0].ARN, nil
		}
		if parsed.NextToken == "" {
			return "", nil
		}
		nextToken = parsed.NextToken
	}
	return "", nil
}
