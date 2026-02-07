//go:build unit

package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtractGeminiCLISessionHash(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		privilegedUserID string
		wantEmpty        bool
		wantHash         string
REDACTED{
		{
			name:             "with privileged-user-id and tmp dir",
			body:             `{"contents":[{"parts":[{"text":"The project's temporary directory is: /Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740"REDACTED]REDACTED]REDACTED`,
			privilegedUserID: "90785f52-8bbe-4b17-b111-a1ddea1636c3",
			wantEmpty:        false,
			wantHash: func() string {
				combined := "90785f52-8bbe-4b17-b111-a1ddea1636c3:f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740"
				hash := sha256.Sum256([]byte(combined))
				return hex.EncodeToString(hash[:])
		REDACTED(),
	REDACTED,
		{
			name:             "without privileged-user-id but with tmp dir",
			body:             `{"contents":[{"parts":[{"text":"The project's temporary directory is: /Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740"REDACTED]REDACTED]REDACTED`,
			privilegedUserID: "",
			wantEmpty:        false,
			wantHash:         "f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
	REDACTED,
		{
			name:             "without tmp dir",
			body:             `{"contents":[{"parts":[{"text":"Hello world"REDACTED]REDACTED]REDACTED`,
			privilegedUserID: "90785f52-8bbe-4b17-b111-a1ddea1636c3",
			wantEmpty:        true,
	REDACTED,
		{
			name:             "empty body",
			body:             "",
			privilegedUserID: "90785f52-8bbe-4b17-b111-a1ddea1636c3",
			wantEmpty:        true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试上下文
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/test", nil)
			if tt.privilegedUserID != "" {
				c.Request.Header.Set("x-gemini-api-privileged-user-id", tt.privilegedUserID)
		REDACTED

			// 调用函数
			result := extractGeminiCLISessionHash(c, []byte(tt.body))

			// 验证结果
			if tt.wantEmpty {
				require.Empty(t, result, "expected empty session hash")
		REDACTED else {
				require.NotEmpty(t, result, "expected non-empty session hash")
				require.Equal(t, tt.wantHash, result, "session hash mismatch")
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGeminiCLITmpDirRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMatch bool
		wantHash  string
REDACTED{
		{
			name:      "valid tmp dir path",
			input:     "/Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
			wantMatch: true,
			wantHash:  "f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
	REDACTED,
		{
			name:      "valid tmp dir path in text",
			input:     "The project's temporary directory is: /Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740\nOther text",
			wantMatch: true,
			wantHash:  "f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
	REDACTED,
		{
			name:      "invalid hash length",
			input:     "/Users/ianshaw/.gemini/tmp/abc123",
			wantMatch: false,
	REDACTED,
		{
			name:      "no tmp dir",
			input:     "Hello world",
			wantMatch: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := geminiCLITmpDirRegex.FindStringSubmatch(tt.input)
			if tt.wantMatch {
				require.NotNil(t, match, "expected regex to match")
				require.Len(t, match, 2, "expected 2 capture groups")
				require.Equal(t, tt.wantHash, match[1], "hash mismatch")
		REDACTED else {
				require.Nil(t, match, "expected regex not to match")
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestSafeShortPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
REDACTED{
		{name: "空字符串", input: "", n: 8, want: ""REDACTED,
		{name: "长度小于截断值", input: "abc", n: 8, want: "abc"REDACTED,
		{name: "长度等于截断值", input: "12345678", n: 8, want: "12345678"REDACTED,
		{name: "长度大于截断值", input: "1234567890", n: 8, want: "12345678"REDACTED,
		{name: "截断值为0", input: "123456", n: 0, want: "123456"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, safeShortPrefix(tt.input, tt.n))
	REDACTED)
REDACTED
REDACTED
