package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubCredRepo 是最小化 AccountRepository stub，仅实现 GetByID，供 credential_shadow_test 使用。
// 嵌入接口满足完整方法集；未实现的方法若被调用会 panic，从而快速暴露误调用。
type stubCredRepo struct {
	AccountRepository
	parent *Account
REDACTED

func (s *stubCredRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return s.parent, nil
REDACTED

func newStubCredRepo(parent *Account) AccountRepository {
	return &stubCredRepo{parent: parentREDACTED
REDACTED

func TestResolveCredentialAccount(t *testing.T) {
	ctx := context.Background()
	pid := int64(100)

	// 普通账号（非影子）→ 返回自身
	parent := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActiveREDACTED
	repo := newStubCredRepo(parent)
	got, err := resolveCredentialAccount(ctx, repo, parent)
REDACTED
	require.Equal(t, int64(100), got.ID)

	// 影子账号 + 合法 OpenAI OAuth 母账号 → 返回母账号
	shadow := &Account{ID: 200, ParentAccountID: &pid, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	got, err = resolveCredentialAccount(ctx, repo, shadow)
REDACTED
	require.Equal(t, int64(100), got.ID)

	// 影子账号 + 母账号非 OpenAI OAuth（API Key 类型）→ 返回 error
	badRepo := newStubCredRepo(&Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED)
	_, err = resolveCredentialAccount(ctx, badRepo, shadow)
REDACTED
REDACTED
