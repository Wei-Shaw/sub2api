//go:build e2e

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type codeBuddyDBFixture struct {
	APIKey    string
	GroupID   int64
	AccountID int64
	Model     string
}

func TestCodeBuddyDatabaseBackedIngressMatrix(t *testing.T) {
	if os.Getenv("CODEBUDDY_DB_E2E") != "1" {
		t.Skip("设置 CODEBUDDY_DB_E2E=1 后运行数据库驱动的 CodeBuddy E2E")
	}

	fixture := loadCodeBuddyDBFixture(t)
	client := &http.Client{Timeout: 90 * time.Second}
	tests := []struct {
		name         string
		path         string
		stream       bool
		payload      map[string]any
		assertResult func(*testing.T, string)
	}{
		{
			name:   "messages_non_stream",
			path:   "/v1/messages",
			stream: false,
			payload: map[string]any{"model": fixture.Model, "max_tokens": 32, "stream": false,
				"messages": []map[string]string{{"role": "user", "content": "Reply with exactly OK"}}},
			assertResult: func(t *testing.T, body string) {
				require.Equal(t, "message", gjson.Get(body, "type").String())
				require.Equal(t, "assistant", gjson.Get(body, "role").String())
			},
		},
		{
			name:   "messages_stream",
			path:   "/v1/messages",
			stream: true,
			payload: map[string]any{"model": fixture.Model, "max_tokens": 32, "stream": true,
				"messages": []map[string]string{{"role": "user", "content": "Reply with exactly OK"}}},
			assertResult: func(t *testing.T, body string) {
				require.Contains(t, body, "event: message_start")
				require.Contains(t, body, "event: message_stop")
			},
		},
		{
			name:   "chat_completions_non_stream",
			path:   "/v1/chat/completions",
			stream: false,
			payload: map[string]any{"model": fixture.Model, "stream": false,
				"messages": []map[string]string{{"role": "user", "content": "Reply with exactly OK"}}},
			assertResult: func(t *testing.T, body string) {
				require.Equal(t, "chat.completion", gjson.Get(body, "object").String())
				require.NotEmpty(t, gjson.Get(body, "choices.0.message").Raw)
			},
		},
		{
			name:   "chat_completions_stream",
			path:   "/v1/chat/completions",
			stream: true,
			payload: map[string]any{"model": fixture.Model, "stream": true,
				"messages": []map[string]string{{"role": "user", "content": "Reply with exactly OK"}}},
			assertResult: func(t *testing.T, body string) {
				require.Contains(t, body, "chat.completion.chunk")
				require.Contains(t, body, "data: [DONE]")
			},
		},
		{
			name:    "responses_non_stream",
			path:    "/v1/responses",
			stream:  false,
			payload: map[string]any{"model": fixture.Model, "input": "Reply with exactly OK", "stream": false},
			assertResult: func(t *testing.T, body string) {
				require.Equal(t, "response", gjson.Get(body, "object").String())
				require.True(t, gjson.Get(body, "output").IsArray())
			},
		},
		{
			name:    "responses_stream",
			path:    "/v1/responses",
			stream:  true,
			payload: map[string]any{"model": fixture.Model, "input": "Reply with exactly OK", "stream": true},
			assertResult: func(t *testing.T, body string) {
				require.Contains(t, body, "event: response.completed")
				require.Contains(t, body, "data: [DONE]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(tt.payload)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, strings.TrimRight(baseURL, "/")+tt.path, bytes.NewReader(payload))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+fixture.APIKey)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equalf(t, http.StatusOK, resp.StatusCode, "response: %s", string(body))
			if tt.stream {
				require.Contains(t, strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
			} else {
				require.Contains(t, strings.ToLower(resp.Header.Get("Content-Type")), "application/json")
			}
			tt.assertResult(t, string(body))
		})
	}
}

func loadCodeBuddyDBFixture(t *testing.T) codeBuddyDBFixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("CODEBUDDY_E2E_DATABASE_DSN"))
	if dsn == "" {
		t.Fatal("CODEBUDDY_E2E_DATABASE_DSN is required when CODEBUDDY_DB_E2E=1")
	}
	apiKeyID := codeBuddyE2EInt64(t, "CODEBUDDY_E2E_API_KEY_ID", 2)
	groupID := codeBuddyE2EInt64(t, "CODEBUDDY_E2E_GROUP_ID", 8)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))

	var fixture codeBuddyDBFixture
	fixture.GroupID = groupID
	var platform string
	err = db.QueryRowContext(ctx, `
SELECT k.key, g.platform
FROM api_keys k
JOIN groups g ON g.id = k.group_id
WHERE k.id = $1 AND k.group_id = $2 AND k.status = 'active' AND g.status = 'active'`, apiKeyID, groupID).Scan(&fixture.APIKey, &platform)
	require.NoError(t, err)
	require.Equal(t, "codebuddy", platform)

	var modelsJSON []byte
	err = db.QueryRowContext(ctx, `
SELECT a.id, a.credentials->'models'
FROM accounts a
JOIN account_groups ag ON ag.account_id = a.id
WHERE ag.group_id = $1 AND a.platform = 'codebuddy' AND a.status = 'active' AND a.schedulable = true
ORDER BY ag.priority DESC, a.priority DESC, a.id
LIMIT 1`, groupID).Scan(&fixture.AccountID, &modelsJSON)
	require.NoError(t, err)

	var models []string
	require.NoError(t, json.Unmarshal(modelsJSON, &models))
	fixture.Model = chooseCodeBuddyE2EModel(models, strings.TrimSpace(os.Getenv("CODEBUDDY_E2E_MODEL")))
	require.NotEmpty(t, fixture.Model, "数据库账号没有可用于 E2E 的 CodeBuddy 模型")
	return fixture
}

func codeBuddyE2EInt64(t *testing.T, envName string, fallback int64) int64 {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	require.NoError(t, err, fmt.Sprintf("invalid %s", envName))
	return value
}

func chooseCodeBuddyE2EModel(models []string, requested string) string {
	available := make(map[string]bool, len(models))
	for _, model := range models {
		available[model] = true
	}
	if requested != "" && available[requested] {
		return requested
	}
	for _, preferred := range []string{"glm-5.2", "hy3", "glm-5.1"} {
		if available[preferred] {
			return preferred
		}
	}
	return ""
}
