//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type updateAccountCredsRepoStub struct {
	mockAccountRepoForGemini
	account     *Account
	updateCalls int
REDACTED

func (r *updateAccountCredsRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
REDACTED

func (r *updateAccountCredsRepoStub) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
REDACTED

func TestUpdateAccount_PreservesSensitiveCredsWhenIncomingOmits(t *testing.T) {
	accountID := int64(202)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
	REDACTED
				"refresh_token": "rt-existing",
				"access_token":  "at-existing",
				"id_token":      "id-existing",
				"base_url":      "https://old.example.com",
		REDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	// 模拟前端编辑：仅修改 base_url，没有传 token（脱敏后前端 spread 拿不到敏感键）
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
REDACTED
			"base_url": "https://new.example.com",
	REDACTED,
REDACTED)

REDACTED
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)

	// 敏感键应保留
	require.Equal(t, "rt-existing", repo.account.Credentials["refresh_token"])
	require.Equal(t, "at-existing", repo.account.Credentials["access_token"])
	require.Equal(t, "id-existing", repo.account.Credentials["id_token"])
	// 非敏感键被替换
	require.Equal(t, "https://new.example.com", repo.account.Credentials["base_url"])
REDACTED

func TestUpdateAccount_ExplicitNewTokenOverwrites(t *testing.T) {
	accountID := int64(203)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
	REDACTED
				"refresh_token": "rt-old",
				"api_key":       "sk-old",
		REDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
REDACTED
			"refresh_token": "rt-new",
			// api_key 没传 → 应保留旧值
	REDACTED,
REDACTED)
REDACTED
	require.NotNil(t, updated)

	require.Equal(t, "rt-new", repo.account.Credentials["refresh_token"])
	require.Equal(t, "sk-old", repo.account.Credentials["api_key"])
REDACTED

func TestUpdateAccount_EmptyCredentialsSkipsUpdate(t *testing.T) {
	accountID := int64(204)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
	REDACTED
				"refresh_token": "rt-existing",
		REDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{accountRepo: repoREDACTED

	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
REDACTEDREDACTED, // len == 0 → 闸门跳过
		Name:        "renamed",
REDACTED)
REDACTED

	require.Equal(t, "rt-existing", repo.account.Credentials["refresh_token"], "空 credentials 不应触碰已有 token")
	require.Equal(t, "renamed", repo.account.Name)
REDACTED
