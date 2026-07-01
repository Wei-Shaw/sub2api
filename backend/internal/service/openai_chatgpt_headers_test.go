package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubChatGPTHeadersRepo 是最小化 AccountRepository stub，仅实现 GetByID，
// 供 TestResolveAndSetOpenAIChatGPTAccountHeaders 使用。
type stubChatGPTHeadersRepo struct {
	AccountRepository
	byID map[int64]*Account
REDACTED

func (r *stubChatGPTHeadersRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	return r.byID[id], nil
REDACTED

func TestResolveAndSetOpenAIChatGPTAccountHeaders(t *testing.T) {
	ctx := context.Background()
	pid := int64(100)

	parentCreds := map[string]any{"chatgpt_account_id": "org-parent"REDACTED
	parent := &Account{
		ID:          100,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Credentials: parentCreds,
REDACTED
	repo := &stubChatGPTHeadersRepo{byID: map[int64]*Account{100: parentREDACTEDREDACTED

	t.Run("shadow_resolves_to_parent_org", func(t *testing.T) {
		shadow := &Account{
			ID:              200,
			ParentAccountID: &pid,
			Platform:        PlatformOpenAI,
			Type:            AccountTypeOAuth,
	REDACTED
		headers := make(http.Header)
		err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, repo, headers, shadow)
	REDACTED
		require.Equal(t, "org-parent", headers.Get("chatgpt-account-id"),
			"影子账号应透传母账号的 chatgpt-account-id")
REDACTED)

	t.Run("normal_account_passthrough", func(t *testing.T) {
		ownCreds := map[string]any{"chatgpt_account_id": "org-own"REDACTED
		normal := &Account{
			ID:          300,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Credentials: ownCreds,
	REDACTED
		headers := make(http.Header)
		err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, repo, headers, normal)
	REDACTED
		require.Equal(t, "org-own", headers.Get("chatgpt-account-id"),
			"普通账号应透传自身的 chatgpt-account-id")
REDACTED)
REDACTED
