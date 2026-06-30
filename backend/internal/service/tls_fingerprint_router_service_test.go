package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/stretchr/testify/require"
)

type tlsFingerprintRouterRepoStub struct {
	routers []*model.TLSFingerprintRouter
}

func (r *tlsFingerprintRouterRepoStub) List(context.Context) ([]*model.TLSFingerprintRouter, error) {
	return r.routers, nil
}

func (r *tlsFingerprintRouterRepoStub) GetByID(_ context.Context, id int64) (*model.TLSFingerprintRouter, error) {
	for _, router := range r.routers {
		if router != nil && router.ID == id {
			return router, nil
		}
	}
	return nil, nil
}

func (r *tlsFingerprintRouterRepoStub) Create(_ context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	router.ID = int64(len(r.routers) + 1)
	r.routers = append(r.routers, router)
	return router, nil
}

func (r *tlsFingerprintRouterRepoStub) Update(_ context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	for i, existing := range r.routers {
		if existing != nil && existing.ID == router.ID {
			r.routers[i] = router
			return router, nil
		}
	}
	r.routers = append(r.routers, router)
	return router, nil
}

func (r *tlsFingerprintRouterRepoStub) Delete(_ context.Context, id int64) error {
	for i, router := range r.routers {
		if router != nil && router.ID == id {
			r.routers = append(r.routers[:i], r.routers[i+1:]...)
			return nil
		}
	}
	return nil
}

func newTLSFingerprintRouterTestService(routers ...*model.TLSFingerprintRouter) *TLSFingerprintRouterService {
	return NewTLSFingerprintRouterService(&tlsFingerprintRouterRepoStub{routers: routers}, nil)
}

func TestTLSFingerprintRouterService_MatchUserAgent(t *testing.T) {
	svc := newTLSFingerprintRouterTestService(&model.TLSFingerprintRouter{
		ID:      1,
		Name:    "客户端路由",
		Enabled: true,
		Rules: []model.TLSFingerprintRouterRule{
			{
				Name:                    "禁用规则不会命中",
				Enabled:                 false,
				MatchType:               model.TLSRouterMatchContains,
				Pattern:                 "opencode",
				TLSFingerprintProfileID: 99,
			},
			{
				Name:                    "Codex 精确匹配",
				Enabled:                 true,
				MatchType:               model.TLSRouterMatchExact,
				Pattern:                 "codex_cli_rs/1.0",
				TLSFingerprintProfileID: 1,
				UpstreamUserAgent:       "codex_cli_rs/9.9",
				UpstreamOriginator:      "codex_cli_rs",
			},
			{
				Name:                    "OpenCode 大小写不敏感",
				Enabled:                 true,
				MatchType:               model.TLSRouterMatchContains,
				Pattern:                 "OpenCode",
				TLSFingerprintProfileID: 2,
			},
			{
				Name:                    "前缀匹配",
				Enabled:                 true,
				MatchType:               model.TLSRouterMatchPrefix,
				Pattern:                 "cursor/",
				TLSFingerprintProfileID: 3,
			},
			{
				Name:                    "正则匹配",
				Enabled:                 true,
				MatchType:               model.TLSRouterMatchRegex,
				Pattern:                 `myapp/\d+`,
				TLSFingerprintProfileID: 4,
			},
		},
	})

	tests := []struct {
		name      string
		ua        string
		wantMatch bool
		wantRule  string
		wantID    int64
	}{
		{name: "按顺序跳过禁用规则后命中 exact", ua: "codex_cli_rs/1.0", wantMatch: true, wantRule: "Codex 精确匹配", wantID: 1},
		{name: "contains 默认大小写不敏感", ua: "opencode/0.9", wantMatch: true, wantRule: "OpenCode 大小写不敏感", wantID: 2},
		{name: "prefix 命中", ua: "cursor/2.0", wantMatch: true, wantRule: "前缀匹配", wantID: 3},
		{name: "regex 命中", ua: "MyApp/123", wantMatch: true, wantRule: "正则匹配", wantID: 4},
		{name: "未命中返回 router 信息", ua: "curl/8.0", wantMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.MatchUserAgent(1, tt.ua)
			require.Equal(t, int64(1), result.RouterID)
			require.Equal(t, "客户端路由", result.RouterName)
			require.Equal(t, tt.wantMatch, result.Matched)
			if tt.wantMatch {
				require.Equal(t, tt.wantRule, result.RuleName)
				require.Equal(t, tt.wantID, result.TLSFingerprintProfileID)
				if tt.wantID == 1 {
					require.Equal(t, "codex_cli_rs/9.9", result.UpstreamUserAgent)
					require.Equal(t, "codex_cli_rs", result.UpstreamOriginator)
				}
			}
		})
	}
}

func TestTLSFingerprintRouterService_MatchUserAgent_CaseSensitive(t *testing.T) {
	svc := newTLSFingerprintRouterTestService(&model.TLSFingerprintRouter{
		ID:      2,
		Name:    "大小写路由",
		Enabled: true,
		Rules: []model.TLSFingerprintRouterRule{
			{
				Name:                    "大小写敏感",
				Enabled:                 true,
				MatchType:               model.TLSRouterMatchContains,
				Pattern:                 "OpenCode",
				CaseSensitive:           true,
				TLSFingerprintProfileID: 7,
			},
		},
	})

	require.False(t, svc.MatchUserAgent(2, "opencode/1.0").Matched)
	require.True(t, svc.MatchUserAgent(2, "OpenCode/1.0").Matched)
}

func TestTLSFingerprintRouterService_ValidateRules(t *testing.T) {
	t.Run("空 pattern 拒绝", func(t *testing.T) {
		router := &model.TLSFingerprintRouter{
			Name: "invalid",
			Rules: []model.TLSFingerprintRouterRule{{
				Enabled:   true,
				MatchType: model.TLSRouterMatchContains,
				Pattern:   "   ",
			}},
		}
		require.Error(t, router.Validate())
	})

	t.Run("非法 regex 拒绝", func(t *testing.T) {
		router := &model.TLSFingerprintRouter{
			Name: "invalid",
			Rules: []model.TLSFingerprintRouterRule{{
				Enabled:   true,
				MatchType: model.TLSRouterMatchRegex,
				Pattern:   "[",
			}},
		}
		require.Error(t, router.Validate())
	})
}

func TestTLSFingerprintRouterService_Create_NormalizesChatGPTOAuthTokenConfig(t *testing.T) {
	profileID := int64(-1)
	repo := &tlsFingerprintRouterRepoStub{}
	svc := NewTLSFingerprintRouterService(repo, nil)

	created, err := svc.Create(context.Background(), &model.TLSFingerprintRouter{
		Name:                                     "  token router  ",
		Enabled:                                  true,
		ChatGPTOAuthTokenUserAgent:               "  codex-token-ua  ",
		ChatGPTOAuthTokenTLSFingerprintProfileID: &profileID,
		Rules:                                    nil,
	})

	require.NoError(t, err)
	require.Equal(t, "token router", created.Name)
	require.Equal(t, "codex-token-ua", created.ChatGPTOAuthTokenUserAgent)
	require.Equal(t, int64(-1), *created.ChatGPTOAuthTokenTLSFingerprintProfileID)
	require.Empty(t, created.Rules)
}

func TestTLSFingerprintRouter_ValidateChatGPTOAuthTokenProfileID(t *testing.T) {
	tests := []struct {
		name    string
		value   *int64
		wantErr bool
	}{
		{name: "未配置", value: nil},
		{name: "内置默认模板", value: tlsRouterInt64Ptr(0)},
		{name: "随机模板", value: tlsRouterInt64Ptr(-1)},
		{name: "指定模板", value: tlsRouterInt64Ptr(123)},
		{name: "非法负数", value: tlsRouterInt64Ptr(-2), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &model.TLSFingerprintRouter{
				Name:                                     "router",
				ChatGPTOAuthTokenTLSFingerprintProfileID: tt.value,
			}
			err := router.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func tlsRouterInt64Ptr(value int64) *int64 {
	return &value
}
