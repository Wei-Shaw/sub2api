//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

// GatewayRoutingSuite 测试网关路由相关的数据库查询
// 验证账户选择和分流逻辑在真实数据库环境下的行为
type GatewayRoutingSuite struct {
	suite.Suite
	ctx         context.Context
	client      *dbent.Client
	accountRepo *accountRepository
}

func (s *GatewayRoutingSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.accountRepo = newAccountRepositoryWithSQL(s.client, tx, nil)
}

func TestGatewayRoutingSuite(t *testing.T) {
	suite.Run(t, new(GatewayRoutingSuite))
}

// TestListSchedulableByPlatforms_GeminiAndAntigravity 验证多平台账户查询
func (s *GatewayRoutingSuite) TestListSchedulableByPlatforms_GeminiAndAntigravity() {
	// 创建各平台账户
	geminiAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "gemini-oauth",
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    1,
	})

	antigravityAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "antigravity-oauth",
		Platform:    service.PlatformAntigravity,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    2,
		Credentials: map[string]any{
			"access_token":  "test-token",
			"refresh_token": "test-refresh",
			"project_id":    "test-project",
		},
	})

	// 创建不应被选中的 anthropic 账户
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "anthropic-oauth",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    0,
	})

	// 查询 gemini + antigravity 平台
	accounts, err := s.accountRepo.ListSchedulableByPlatforms(s.ctx, []string{
		service.PlatformGemini,
		service.PlatformAntigravity,
	})

	s.Require().NoError(err)
	s.Require().Len(accounts, 2, "应返回 gemini 和 antigravity 两个账户")

	// 验证返回的账户平台
	platforms := make(map[string]bool)
	for _, acc := range accounts {
		platforms[acc.Platform] = true
	}
	s.Require().True(platforms[service.PlatformGemini], "应包含 gemini 账户")
	s.Require().True(platforms[service.PlatformAntigravity], "应包含 antigravity 账户")
	s.Require().False(platforms[service.PlatformAnthropic], "不应包含 anthropic 账户")

	// 验证账户 ID 匹配
	ids := make(map[int64]bool)
	for _, acc := range accounts {
		ids[acc.ID] = true
	}
	s.Require().True(ids[geminiAcc.ID])
	s.Require().True(ids[antigravityAcc.ID])
}

// TestListSchedulableByGroupIDAndPlatforms_WithGroupBinding 验证按分组过滤
func (s *GatewayRoutingSuite) TestListSchedulableByGroupIDAndPlatforms_WithGroupBinding() {
	// 创建 gemini 分组
	group := mustCreateGroup(s.T(), s.client, &service.Group{
		Name:     "gemini-group",
		Platform: service.PlatformGemini,
		Status:   service.StatusActive,
	})

	// 创建账户
	boundAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "bound-antigravity",
		Platform:    service.PlatformAntigravity,
		Status:      service.StatusActive,
		Schedulable: true,
	})
	unboundAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "unbound-antigravity",
		Platform:    service.PlatformAntigravity,
		Status:      service.StatusActive,
		Schedulable: true,
	})

	// 只绑定一个账户到分组
	mustBindAccountToGroup(s.T(), s.client, boundAcc.ID, group.ID, 1)

	// 查询分组内的账户
	accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatforms(s.ctx, group.ID, []string{
		service.PlatformGemini,
		service.PlatformAntigravity,
	})

	s.Require().NoError(err)
	s.Require().Len(accounts, 1, "应只返回绑定到分组的账户")
	s.Require().Equal(boundAcc.ID, accounts[0].ID)

	// 确认未绑定的账户不在结果中
	for _, acc := range accounts {
		s.Require().NotEqual(unboundAcc.ID, acc.ID, "不应包含未绑定的账户")
	}
}

// TestListSchedulableByPlatform_Antigravity 验证单平台查询
func (s *GatewayRoutingSuite) TestListSchedulableByPlatform_Antigravity() {
	// 创建多种平台账户
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "gemini-1",
		Platform:    service.PlatformGemini,
		Status:      service.StatusActive,
		Schedulable: true,
	})

	antigravity := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "antigravity-1",
		Platform:    service.PlatformAntigravity,
		Status:      service.StatusActive,
		Schedulable: true,
	})

	// 只查询 antigravity 平台
	accounts, err := s.accountRepo.ListSchedulableByPlatform(s.ctx, service.PlatformAntigravity)

	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(antigravity.ID, accounts[0].ID)
	s.Require().Equal(service.PlatformAntigravity, accounts[0].Platform)
}

// TestSchedulableFilter_ExcludesInactive 验证不可调度账户被过滤
func (s *GatewayRoutingSuite) TestSchedulableFilter_ExcludesInactive() {
	// 创建可调度账户
	activeAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "active-antigravity",
		Platform:    service.PlatformAntigravity,
		Status:      service.StatusActive,
		Schedulable: true,
	})

	// 创建不可调度账户（需要先创建再更新，因为 fixture 默认设置 Schedulable=true）
	inactiveAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "inactive-antigravity",
		Platform: service.PlatformAntigravity,
		Status:   service.StatusActive,
	})
	s.Require().NoError(s.client.Account.UpdateOneID(inactiveAcc.ID).SetSchedulable(false).Exec(s.ctx))

	// 创建错误状态账户
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "error-antigravity",
		Platform:    service.PlatformAntigravity,
		Status:      service.StatusError,
		Schedulable: true,
	})

	accounts, err := s.accountRepo.ListSchedulableByPlatform(s.ctx, service.PlatformAntigravity)

	s.Require().NoError(err)
	s.Require().Len(accounts, 1, "应只返回可调度的 active 账户")
	s.Require().Equal(activeAcc.ID, accounts[0].ID)
}

// TestPlatformRoutingDecision 验证平台路由决策
// 这个测试模拟 Handler 层在选择账户后的路由决策逻辑
func (s *GatewayRoutingSuite) TestPlatformRoutingDecision() {
	// 创建两种平台的账户
	geminiAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "gemini-route-test",
		Platform:    service.PlatformGemini,
		Status:      service.StatusActive,
		Schedulable: true,
	})

	antigravityAcc := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "antigravity-route-test",
		Platform:    service.PlatformAntigravity,
		Status:      service.StatusActive,
		Schedulable: true,
	})

	tests := []struct {
		name            string
		accountID       int64
		expectedService string
	}{
		{
			name:            "Gemini账户路由到ForwardNative",
			accountID:       geminiAcc.ID,
			expectedService: "GeminiMessagesCompatService.ForwardNative",
		},
		{
			name:            "Antigravity账户路由到ForwardGemini",
			accountID:       antigravityAcc.ID,
			expectedService: "AntigravityGatewayService.ForwardGemini",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 从数据库获取账户
			account, err := s.accountRepo.GetByID(s.ctx, tt.accountID)
			s.Require().NoError(err)

			// 模拟 Handler 层的路由决策
			var routedService string
			if account.Platform == service.PlatformAntigravity {
				routedService = "AntigravityGatewayService.ForwardGemini"
			} else {
				routedService = "GeminiMessagesCompatService.ForwardNative"
			}

			s.Require().Equal(tt.expectedService, routedService)
		})
	}
}
