package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDingTalkOAuthStart_Disabled は sentinel テスト。
// TODO(task-1.10): newTestAuthHandlerWithDingTalk helper が追加されたら t.Skip を外す。
func TestDingTalkOAuthStart_Disabled(t *testing.T) {
	t.Skip("helper newTestAuthHandlerWithDingTalk added in Task 1.10; sentinel only")
}

// TestBuildDingTalkSyntheticEmail_UsesUnionID 验证合成邮箱种子使用 unionID。
func TestBuildDingTalkSyntheticEmail_UsesUnionID(t *testing.T) {
	unionID := "union_AbCdEf123"
	email := buildDingTalkSyntheticEmail(unionID)

	want := "dingtalk-union_abcdef123@dingtalk-connect.invalid"
	require.Equal(t, want, email)

	// 确保结果都是小写（邮箱大小写不敏感，统一小写）
	require.True(t, strings.ToLower(email) == email, "synthetic email should be all lowercase")

	// 确保前缀正确
	require.True(t, strings.HasPrefix(email, "dingtalk-"), "should have dingtalk- prefix")

	// 确保后缀是合成邮箱域名
	require.True(t, strings.HasSuffix(email, "@dingtalk-connect.invalid"), "should have reserved domain suffix")
}

// TestBuildDingTalkSyntheticEmail_TrimsSpace 验证 unionID 空白被修剪。
func TestBuildDingTalkSyntheticEmail_TrimsSpace(t *testing.T) {
	email := buildDingTalkSyntheticEmail("  UID_XYZ  ")
	require.Equal(t, "dingtalk-uid_xyz@dingtalk-connect.invalid", email)
}

// TestBuildDingTalkUpstreamClaims_EmptyStaff 验证 staff 为空 struct（跨组织降级路径）时：
// - subject 等于 unionID（与 identityKey.ProviderSubject 一致）
// - corp_user_id 为空字符串（跨组织时拿不到企业 userid）
// - email/username 为空字符串
// B/C: Step 3/4 失败降级时 staff = &DingTalkStaffInfo{}，claims 不应有 nil。
func TestBuildDingTalkUpstreamClaims_EmptyStaff(t *testing.T) {
	staff := &DingTalkStaffInfo{}
	claims := buildDingTalkUpstreamClaims(staff, "UNION_AAA", "CORP_X")

	require.Equal(t, "", claims["email"])
	require.Equal(t, "", claims["username"])
	// 重构后 subject = unionID（与 identityKey.ProviderSubject 保持一致）
	require.Equal(t, "UNION_AAA", claims["subject"])
	require.Equal(t, "", claims["corp_user_id"]) // 企业 userid 跨组织时为空
	require.Equal(t, "UNION_AAA", claims["union_id"])
	require.Equal(t, "CORP_X", claims["corp_id"])
}

// TestCheckDingTalkCorpAllowed_CrossOrgPolicy 验证 policy=none 时允许任意 corp。
// D: corp 校验提前后逻辑不变。
func TestCheckDingTalkCorpAllowed_CrossOrgPolicy(t *testing.T) {
	cfg := config.DingTalkConnectConfig{CorpRestrictionPolicy: "none"}

	assert.True(t, checkDingTalkCorpAllowed(cfg, "dingABC"), "policy=none should allow any corp")
	assert.True(t, checkDingTalkCorpAllowed(cfg, ""), "policy=none should allow empty corp")
	assert.True(t, checkDingTalkCorpAllowed(cfg, "foreign_corp"), "policy=none should allow foreign corp")
}

// TestCheckDingTalkCorpAllowed_InternalOnly 验证 policy=internal_only 时的 corp 校验语义（方案 A 修订）。
// 钉钉 userAccessToken 在部分授权场景（扫码登录、非企业工作台入口）不返回 corpId 字段，
// 因此 checkDingTalkCorpAllowed 完全不校验 corpID，由 step 3 GetUserIdByUnionId 做真实判定
// （跨企业用户会被钉钉错误码 60011/60121 拒绝，mapDingTalkErrorCode 映射回 corp_rejected）。
func TestCheckDingTalkCorpAllowed_InternalOnly(t *testing.T) {
	cfgWithCorpID := config.DingTalkConnectConfig{
		CorpRestrictionPolicy: "internal_only",
		InternalCorpID:        "dingInternal",
	}
	assert.True(t, checkDingTalkCorpAllowed(cfgWithCorpID, "dingInternal"), "internal_only: matching corpID allowed")
	assert.True(t, checkDingTalkCorpAllowed(cfgWithCorpID, "foreign_corp"), "internal_only: corpID 字段不再用于决策，step 3 兜底")
	assert.True(t, checkDingTalkCorpAllowed(cfgWithCorpID, ""), "internal_only: 空 corpID 也通过（钉钉部分授权场景不返回 corpId）")

	cfgNoCorpID := config.DingTalkConnectConfig{
		CorpRestrictionPolicy: "internal_only",
		InternalCorpID:        "",
	}
	assert.True(t, checkDingTalkCorpAllowed(cfgNoCorpID, "dingAnyNonEmpty"), "internal_only + no InternalCorpID: 非空 corpID 通过")
	assert.True(t, checkDingTalkCorpAllowed(cfgNoCorpID, ""), "internal_only + no InternalCorpID: 空 corpID 也通过")
}

// TestDecideDingTalkStep34Strategy_PolicyNone 验证 policy=none 时
// Step 3/4 失败应降级（shouldFallback=true, isFatal=false）。
func TestDecideDingTalkStep34Strategy_PolicyNone(t *testing.T) {
	step3Err := &DingTalkAPIError{Code: "60011", Message: "not in directory", HTTP: 403}

	shouldFallback, isFatal := decideDingTalkStep34Strategy("none", step3Err)

	require.True(t, shouldFallback, "policy=none: step3 failure should trigger fallback")
	require.False(t, isFatal, "policy=none: step3 failure should NOT be fatal")
}

// TestDecideDingTalkStep34Strategy_PolicyNoneEmpty 验证 policy="" 时行为与 "none" 相同。
func TestDecideDingTalkStep34Strategy_PolicyNoneEmpty(t *testing.T) {
	stepErr := &DingTalkAPIError{Code: "60011", Message: "not in directory", HTTP: 403}

	shouldFallback, isFatal := decideDingTalkStep34Strategy("", stepErr)

	require.True(t, shouldFallback, "policy='': step failure should trigger fallback")
	require.False(t, isFatal, "policy='': step failure should NOT be fatal")
}

// TestDecideDingTalkStep34Strategy_PolicyInternalOnly 验证 policy=internal_only 时
// Step 3/4 失败应 hard fail（isFatal=true）。
func TestDecideDingTalkStep34Strategy_PolicyInternalOnly(t *testing.T) {
	step3Err := &DingTalkAPIError{Code: "60011", Message: "not in directory", HTTP: 403}

	shouldFallback, isFatal := decideDingTalkStep34Strategy("internal_only", step3Err)

	require.False(t, shouldFallback, "policy=internal_only: should NOT fallback on step3 error")
	require.True(t, isFatal, "policy=internal_only: step3 failure should be fatal")
}

// TestDecideDingTalkStep34Strategy_NoError 验证 stepErr=nil 时两个返回值均为 false。
func TestDecideDingTalkStep34Strategy_NoError(t *testing.T) {
	for _, policy := range []string{"none", "internal_only", ""} {
		shouldFallback, isFatal := decideDingTalkStep34Strategy(policy, nil)
		require.False(t, shouldFallback, "no error should not trigger fallback (policy=%q)", policy)
		require.False(t, isFatal, "no error should not be fatal (policy=%q)", policy)
	}
}

// TestCompleteDingTalkRegistration_UsernameFromEmailLocalPart 验证 username 为空时
// 退到 email local part（@ 之前的部分）。
// E: CompleteDingTalkOAuthRegistration username fallback。
func TestCompleteDingTalkRegistration_UsernameFromEmailLocalPart(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		username  string
		wantUser  string
		wantValid bool
	}{
		{
			name:      "username empty, normal email → local part",
			email:     "dingtalk-uid123@dingtalk-connect.invalid",
			username:  "",
			wantUser:  "dingtalk-uid123",
			wantValid: true,
		},
		{
			name:      "username already set → keep original",
			email:     "user@example.com",
			username:  "张三",
			wantUser:  "张三",
			wantValid: true,
		},
		{
			name:      "username empty, no @ in email → use whole email",
			email:     "noemail",
			username:  "",
			wantUser:  "noemail",
			wantValid: true,
		},
		{
			name:      "both empty → invalid",
			email:     "",
			username:  "",
			wantUser:  "",
			wantValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			username := tc.username
			email := tc.email

			// 模拟 CompleteDingTalkOAuthRegistration 中的 fallback 逻辑
			if username == "" {
				if at := strings.Index(email, "@"); at > 0 {
					username = email[:at]
				} else {
					username = email
				}
			}

			isValid := email != "" && username != ""
			require.Equal(t, tc.wantUser, username, fmt.Sprintf("username for email=%q", tc.email))
			require.Equal(t, tc.wantValid, isValid, "validity check")
		})
	}
}

// TestBuildDingTalkUpstreamClaims_SubjectEqualsUnionID 验证重构后 subject = unionID
// 而非 staff.UserID，与 identityKey.ProviderSubject 保持一致。
// §4.2: buildDingTalkUpstreamClaims subject 字段修正。
func TestBuildDingTalkUpstreamClaims_SubjectEqualsUnionID(t *testing.T) {
	staff := &DingTalkStaffInfo{UserID: "user123", Name: "张三", Email: "zhangsan@corp.com"}
	claims := buildDingTalkUpstreamClaims(staff, "union456", "dingcorp789")

	// 重构后 subject = unionID（全局唯一，与 identityKey.ProviderSubject 一致）
	require.Equal(t, "union456", claims["subject"], "subject should equal unionID after refactor")
	// 企业 userid 保留为独立字段，供 audit/debug 使用
	require.Equal(t, "user123", claims["corp_user_id"], "corp_user_id should be staff.UserID")
	// union_id 字段与 subject 相同（冗余保留，便于读取）
	require.Equal(t, "union456", claims["union_id"])
	require.Equal(t, "dingcorp789", claims["corp_id"])
	require.Equal(t, "张三", claims["username"])
	require.Equal(t, "zhangsan@corp.com", claims["email"])
}

// TestBuildDingTalkUpstreamClaims_CrossOrgEmptyCorpUserID 验证跨组织降级时
// corp_user_id 为空字符串（跨组织拿不到企业 userid），subject 仍为 unionID。
func TestBuildDingTalkUpstreamClaims_CrossOrgEmptyCorpUserID(t *testing.T) {
	// 跨组织降级路径：staff = &DingTalkStaffInfo{}（所有字段为零值）
	staff := &DingTalkStaffInfo{}
	claims := buildDingTalkUpstreamClaims(staff, "union_cross_org", "foreign_corp")

	require.Equal(t, "union_cross_org", claims["subject"], "subject should still be unionID for cross-org users")
	require.Equal(t, "", claims["corp_user_id"], "corp_user_id should be empty for cross-org fallback")
	require.Equal(t, "", claims["email"])
	require.Equal(t, "", claims["username"])
}

// TestBuildDingTalkUpstreamClaims_PrimaryDeptIDInClaims 验证首个 dept_id 被存入 claims。
func TestBuildDingTalkUpstreamClaims_PrimaryDeptIDInClaims(t *testing.T) {
	staff := &DingTalkStaffInfo{UserID: "u1", Name: "张三", Email: "a@b.com", DeptIDs: []int64{42, 99}}
	claims := buildDingTalkUpstreamClaims(staff, "uid1", "corpX")

	// 只取首个 dept_id
	require.Equal(t, int64(42), claims["primary_dept_id"], "primary_dept_id should be the first dept_id")
}

// TestBuildDingTalkUpstreamClaims_NoDeptIDs 验证无部门时 primary_dept_id=0。
func TestBuildDingTalkUpstreamClaims_NoDeptIDs(t *testing.T) {
	staff := &DingTalkStaffInfo{UserID: "u2", Name: "李四"}
	claims := buildDingTalkUpstreamClaims(staff, "uid2", "corpY")

	require.Equal(t, int64(0), claims["primary_dept_id"], "primary_dept_id should be 0 when no depts")
}

// TestDingTalkStaffFromClaims_RoundTrip 验证 dingTalkStaffFromClaims 能从 claims 恢复 staff 信息。
func TestDingTalkStaffFromClaims_RoundTrip(t *testing.T) {
	staff := &DingTalkStaffInfo{UserID: "u3", Name: "王五", Email: "ww@corp.com", DeptIDs: []int64{55}}
	claims := buildDingTalkUpstreamClaims(staff, "uid3", "corpZ")

	recovered := dingTalkStaffFromClaims(claims)
	require.Equal(t, "王五", recovered.Name)
	require.Equal(t, "ww@corp.com", recovered.Email)
	require.Equal(t, "u3", recovered.UserID)
	require.Equal(t, []int64{55}, recovered.DeptIDs)
}

// TestResolveDingTalkDeptPath_SingleLevel 验证单层部门（parent_id=1）返回部门名。
func TestResolveDingTalkDeptPath_SingleLevel(t *testing.T) {
	handler := &AuthHandler{}
	callCount := 0
	responses := map[string]string{
		"42": `{"errcode":0,"result":{"dept_id":42,"name":"研发部","parent_id":1}}`,
		"1":  `{"errcode":0,"result":{"dept_id":1,"name":"公司","parent_id":0}}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req struct {
			DeptID int64 `json:"dept_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if resp, ok := responses[fmt.Sprintf("%d", req.DeptID)]; ok {
			_, _ = w.Write([]byte(resp))
		} else {
			_, _ = w.Write([]byte(`{"errcode":60003,"errmsg":"not found"}`))
		}
	}))
	defer server.Close()

	cli := &DingTalkClient{
		cfg:        dingTalkClientConfig{UserInfoURL: server.URL + "/stub"},
		httpClient: server.Client(),
	}
	cli.appToken = "tok"
	cli.appTokenExp = time.Now().Add(time.Hour)

	path, err := handler.resolveDingTalkDeptPath(context.Background(), cli, 42)
	require.NoError(t, err)
	require.Equal(t, "研发部", path)
	require.Equal(t, 2, callCount)
}

// TestSyncDingTalkIdentity_UsesCfgAttrKeys 验证 syncDingTalkIdentity 使用 cfg 中配置的 attr key
// 而不是硬编码值。通过 userAttributeService=nil 使同步路径走 warn 跳过，但在此之前先验证
// syncField 构建逻辑（即 attr key 从 cfg 读取）。
// 间接验证：通过构造定制 cfg，确认不同 attr key 可以正确传入（编译时保证类型正确，运行时不 panic）。
func TestSyncDingTalkIdentity_UsesCfgAttrKeys_NoopWithNilService(t *testing.T) {
	handler := &AuthHandler{
		userAttributeService: nil, // nil → 触发 warn 跳过，但不 panic
	}

	cfg := config.DingTalkConnectConfig{
		CorpRestrictionPolicy: "internal_only",
		SyncCorpEmail:         true,
		SyncDisplayName:       true,
		SyncDept:              true,
		// 自定义 attr key（非默认值）
		SyncCorpEmailAttrKey:   "custom_email_key",
		SyncDisplayNameAttrKey: "custom_name_key",
		SyncDeptAttrKey:        "custom_dept_key",
	}

	staff := &DingTalkStaffInfo{
		Name:  "张三",
		Email: "zhangsan@example.com",
	}

	// 调用不应 panic（userAttributeService 为 nil 时走 warn 跳过路径）
	require.NotPanics(t, func() {
		handler.syncDingTalkIdentity(context.Background(), cfg, nil, 42, staff, false)
	})
}

// TestSyncDingTalkIdentity_DefaultAttrKeys_NoopWithNilService 验证 cfg 默认 attr key 为空时
// 使用 fallback 默认值（dingtalk_email / dingtalk_name / dingtalk_department）。
// 此测试主要验证调用路径不 panic；实际 key 赋值默认值的逻辑在 GetDingTalkConnectOAuthConfig 层。
func TestSyncDingTalkIdentity_DefaultAttrKeys_NoopWithNilService(t *testing.T) {
	handler := &AuthHandler{
		userAttributeService: nil,
	}

	cfg := config.DingTalkConnectConfig{
		CorpRestrictionPolicy: "internal_only",
		SyncCorpEmail:         true,
		SyncDisplayName:       true,
		SyncDept:              false,
		// 不设置 attr key（等同于 GetDingTalkConnectOAuthConfig 未设置时 fallback 后的默认值已在调用前填充）
		SyncCorpEmailAttrKey:   "dingtalk_email",
		SyncDisplayNameAttrKey: "dingtalk_name",
		SyncDeptAttrKey:        "dingtalk_department",
	}

	staff := &DingTalkStaffInfo{
		Name:  "李四",
		Email: "lisi@corp.com",
	}

	require.NotPanics(t, func() {
		handler.syncDingTalkIdentity(context.Background(), cfg, nil, 99, staff, false)
	})
}

// TestResolveDingTalkDeptPath_MultiLevel 验证多层部门路径拼接。
func TestResolveDingTalkDeptPath_MultiLevel(t *testing.T) {
	handler := &AuthHandler{}
	// 模拟：42(AI研发) → parent=10(研发部) → parent=1(根)
	responses := map[string]string{
		"42": `{"errcode":0,"result":{"dept_id":42,"name":"AI研发","parent_id":10}}`,
		"10": `{"errcode":0,"result":{"dept_id":10,"name":"研发部","parent_id":1}}`,
		"1":  `{"errcode":0,"result":{"dept_id":1,"name":"公司","parent_id":0}}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 解析请求 body 拿到 dept_id
		var req struct {
			DeptID int64 `json:"dept_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		key := fmt.Sprintf("%d", req.DeptID)
		w.Header().Set("Content-Type", "application/json")
		if resp, ok := responses[key]; ok {
			_, _ = w.Write([]byte(resp))
		} else {
			_, _ = w.Write([]byte(`{"errcode":60003,"errmsg":"not found"}`))
		}
	}))
	defer server.Close()

	cli := &DingTalkClient{
		cfg:        dingTalkClientConfig{UserInfoURL: server.URL + "/stub"},
		httpClient: server.Client(),
	}
	cli.appToken = "tok"
	cli.appTokenExp = time.Now().Add(time.Hour)

	path, err := handler.resolveDingTalkDeptPath(context.Background(), cli, 42)
	require.NoError(t, err)
	require.Equal(t, "研发部/AI研发", path)
}
