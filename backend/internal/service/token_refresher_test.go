//go:build unit

package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaudeTokenRefresher_NeedsRefresh(t *testing.T) {
	refresher := &ClaudeTokenRefresher{REDACTED
	refreshWindow := 30 * time.Minute

	tests := []struct {
		name        string
		credentials map[string]any
		wantRefresh bool
REDACTED{
		{
			name: "expires_at as string - expired",
			credentials: map[string]any{
				"expires_at": "1000", // 1970-01-01 00:16:40 UTC, 已过期
		REDACTED,
			wantRefresh: true,
	REDACTED,
		{
			name: "expires_at as float64 - expired",
			credentials: map[string]any{
				"expires_at": float64(1000), // 数字类型，已过期
		REDACTED,
			wantRefresh: true,
	REDACTED,
		{
			name: "expires_at as RFC3339 - expired",
			credentials: map[string]any{
				"expires_at": "1970-01-01T00:00:00Z", // RFC3339 格式，已过期
		REDACTED,
			wantRefresh: true,
	REDACTED,
		{
			name: "expires_at as string - far future",
			credentials: map[string]any{
				"expires_at": "9999999999", // 远未来
		REDACTED,
			wantRefresh: false,
	REDACTED,
		{
			name: "expires_at as float64 - far future",
			credentials: map[string]any{
				"expires_at": float64(9999999999), // 远未来，数字类型
		REDACTED,
			wantRefresh: false,
	REDACTED,
		{
			name: "expires_at as RFC3339 - far future",
			credentials: map[string]any{
				"expires_at": "2099-12-31T23:59:59Z", // RFC3339 格式，远未来
		REDACTED,
			wantRefresh: false,
	REDACTED,
		{
			name:        "expires_at missing",
			credentials: map[string]any{REDACTED,
			wantRefresh: false,
	REDACTED,
		{
			name: "expires_at is nil",
			credentials: map[string]any{
				"expires_at": nil,
		REDACTED,
			wantRefresh: false,
	REDACTED,
		{
			name: "expires_at is invalid string",
			credentials: map[string]any{
				"expires_at": "invalid",
		REDACTED,
			wantRefresh: false,
	REDACTED,
		{
			name:        "credentials is nil",
			credentials: nil,
			wantRefresh: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
		REDACTED

			got := refresher.NeedsRefresh(account, refreshWindow)
			require.Equal(t, tt.wantRefresh, got)
	REDACTED)
REDACTED
REDACTED

func TestClaudeTokenRefresher_NeedsRefresh_WithinWindow(t *testing.T) {
	refresher := &ClaudeTokenRefresher{REDACTED
	refreshWindow := 30 * time.Minute

	// 设置一个在刷新窗口内的时间（当前时间 + 15分钟）
	expiresAt := time.Now().Add(15 * time.Minute).Unix()

	tests := []struct {
		name        string
		credentials map[string]any
REDACTED{
		{
			name: "string type - within refresh window",
			credentials: map[string]any{
				"expires_at": strconv.FormatInt(expiresAt, 10),
		REDACTED,
	REDACTED,
		{
			name: "float64 type - within refresh window",
			credentials: map[string]any{
				"expires_at": float64(expiresAt),
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
		REDACTED

			got := refresher.NeedsRefresh(account, refreshWindow)
			require.True(t, got, "should need refresh when within window")
	REDACTED)
REDACTED
REDACTED

func TestClaudeTokenRefresher_NeedsRefresh_OutsideWindow(t *testing.T) {
	refresher := &ClaudeTokenRefresher{REDACTED
	refreshWindow := 30 * time.Minute

	// 设置一个在刷新窗口外的时间（当前时间 + 1小时）
	expiresAt := time.Now().Add(1 * time.Hour).Unix()

	tests := []struct {
		name        string
		credentials map[string]any
REDACTED{
		{
			name: "string type - outside refresh window",
			credentials: map[string]any{
				"expires_at": strconv.FormatInt(expiresAt, 10),
		REDACTED,
	REDACTED,
		{
			name: "float64 type - outside refresh window",
			credentials: map[string]any{
				"expires_at": float64(expiresAt),
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
		REDACTED

			got := refresher.NeedsRefresh(account, refreshWindow)
			require.False(t, got, "should not need refresh when outside window")
	REDACTED)
REDACTED
REDACTED

func TestClaudeTokenRefresher_CanRefresh(t *testing.T) {
	refresher := &ClaudeTokenRefresher{REDACTED

	tests := []struct {
		name     string
		platform string
		accType  string
		want     bool
REDACTED{
		{
			name:     "anthropic oauth - can refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeOAuth,
			want:     true,
	REDACTED,
		{
			name:     "anthropic setup-token - can refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeSetupToken,
			want:     true,
	REDACTED,
		{
			name:     "anthropic api-key - cannot refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeAPIKey,
			want:     false,
	REDACTED,
		{
			name:     "openai oauth - cannot refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeOAuth,
			want:     false,
	REDACTED,
		{
			name:     "gemini oauth - cannot refresh",
			platform: PlatformGemini,
			accType:  AccountTypeOAuth,
			want:     false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Type:     tt.accType,
		REDACTED

			got := refresher.CanRefresh(account)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestOpenAITokenRefresher_CanRefresh(t *testing.T) {
	refresher := &OpenAITokenRefresher{REDACTED

	tests := []struct {
		name     string
		platform string
		accType  string
		want     bool
REDACTED{
		{
			name:     "openai oauth - can refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeOAuth,
			want:     true,
	REDACTED,
		{
			name:     "openai apikey - cannot refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeAPIKey,
			want:     false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Type:     tt.accType,
		REDACTED
			require.Equal(t, tt.want, refresher.CanRefresh(account))
	REDACTED)
REDACTED
REDACTED
