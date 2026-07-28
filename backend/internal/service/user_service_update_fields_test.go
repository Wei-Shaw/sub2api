//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 这些用例锁死"每个入口只声明自己真正要改的列"：
// 任何退回整行回写的改动都会让并发写入被陈旧快照覆盖，并在这里变红。

func TestUpdateProfile_OnlyDeclaresRequestedColumns(t *testing.T) {
	username := "renamed"
	tests := []struct {
		name string
		req  UpdateProfileRequest
		want UserUpdateFields
REDACTED{
		{
			name: "username only",
			req:  UpdateProfileRequest{Username: &usernameREDACTED,
			want: UserUpdateFields{Username: trueREDACTED,
	REDACTED,
		{
			name: "notify settings only",
			req:  UpdateProfileRequest{BalanceNotifyEnabled: boolPtr(true)REDACTED,
			want: UserUpdateFields{BalanceNotifySettings: trueREDACTED,
	REDACTED,
		{
			name: "username and notify threshold",
			req:  UpdateProfileRequest{Username: &username, BalanceNotifyThreshold: float64Ptr(1.5)REDACTED,
			want: UserUpdateFields{Username: true, BalanceNotifySettings: trueREDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUserRepo{getByIDUser: &User{ID: 7, Balance: 0.30, Status: StatusActiveREDACTEDREDACTED
			svc := NewUserService(repo, nil, nil, nil)

			_, err := svc.UpdateProfile(context.Background(), 7, tt.req)
		REDACTED
			require.Equal(t, []UserUpdateFields{tt.wantREDACTED, repo.updateFields)
	REDACTED)
REDACTED
REDACTED

// 只改头像时用户行没有任何列要写，不应产生一次整行更新。
func TestUpdateProfile_AvatarOnlySkipsUserRowWrite(t *testing.T) {
	repo := &mockUserRepo{getByIDUser: &User{ID: 7, Balance: 0.30REDACTEDREDACTED
	svc := NewUserService(repo, nil, nil, nil)

	avatar := "https://cdn.example.com/a.png"
	_, err := svc.UpdateProfile(context.Background(), 7, UpdateProfileRequest{AvatarURL: &avatarREDACTED)
REDACTED
	require.Len(t, repo.upsertAvatarArgs, 1, "avatar must still be stored")
	require.Equal(t, []UserUpdateFields{{REDACTEDREDACTED, repo.updateFields, "no user column should be declared")
REDACTED

func TestChangePassword_OnlyDeclaresPasswordHash(t *testing.T) {
	user := &User{ID: 7, Balance: 0.30REDACTED
	require.NoError(t, user.SetPassword("old-password"))
	repo := &mockUserRepo{getByIDUser: userREDACTED
	svc := NewUserService(repo, nil, nil, nil)

	err := svc.ChangePassword(context.Background(), 7, ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     "new-password",
REDACTED)
REDACTED
	require.Equal(t, []UserUpdateFields{{PasswordHash: trueREDACTEDREDACTED, repo.updateFields)
REDACTED

func TestUpdateStatus_OnlyDeclaresStatus(t *testing.T) {
	repo := &mockUserRepo{getByIDUser: &User{ID: 7, Balance: 0.30, Status: StatusActiveREDACTEDREDACTED
	svc := NewUserService(repo, nil, nil, nil)

	require.NoError(t, svc.UpdateStatus(context.Background(), 7, StatusDisabled))
	require.Equal(t, []UserUpdateFields{{Status: trueREDACTEDREDACTED, repo.updateFields)
REDACTED
