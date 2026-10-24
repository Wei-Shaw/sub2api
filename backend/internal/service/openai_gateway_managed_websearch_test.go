//go:build unit

package service

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type managedSearchAccountRepo struct {
	AccountRepository
	account      *Account
	groupAccount []Account
}

func (r *managedSearchAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *managedSearchAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return r.groupAccount, nil
}

func setManagedWebSearchConfigForTest(id int64, enabled bool) {
	var managedID *int64
	if id > 0 {
		managedID = &id
	}
	webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
		config: &WebSearchEmulationConfig{
			Enabled:          enabled,
			ManagedAccountID: managedID,
		},
		expiresAt: time.Now().Add(time.Minute).UnixNano(),
	})
}

func TestManagedWebSearchAccount(t *testing.T) {
	account := &Account{
		ID:       918,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "test-key",
			"api_protocol": APIProtocolAnthropic,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:    &managedSearchAccountRepo{account: account},
		settingService: &SettingService{},
	}
	body := []byte(`{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`)

	setManagedWebSearchConfigForTest(918, true)
	require.Same(t, account, svc.ManagedWebSearchAccount(context.Background(), nil, body))

	setManagedWebSearchConfigForTest(918, false)
	require.Same(t, account, svc.ManagedWebSearchAccount(context.Background(), nil, body))

	setManagedWebSearchConfigForTest(0, true)
	require.Nil(t, svc.ManagedWebSearchAccount(context.Background(), nil, body))

	setManagedWebSearchConfigForTest(918, true)
	require.Nil(t, svc.ManagedWebSearchAccount(context.Background(), nil, []byte(`{"tools":[{"type":"function","name":"lookup"}]}`)))

	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	require.Nil(t, svc.ManagedWebSearchAccount(context.Background(), nil, body))
}

func TestManagedWebSearchAccountUsesCapabilityFlag(t *testing.T) {
	groupID := int64(7)
	capable := &Account{
		ID:       919,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{extraKeyWebSearchCapable: true},
		Credentials: map[string]any{
			"api_key":      "test-key",
			"api_protocol": APIProtocolAdaptive,
		},
	}
	chatOnly := &Account{
		ID:       920,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{extraKeyWebSearchCapable: true},
		Credentials: map[string]any{
			"api_key":      "test-key",
			"api_protocol": APIProtocolChatCompletions,
		},
	}
	repo := &managedSearchAccountRepo{
		groupAccount: []Account{*chatOnly, *capable},
	}
	svc := &OpenAIGatewayService{
		accountRepo:    repo,
		settingService: &SettingService{},
	}
	body := []byte(`{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`)

	setManagedWebSearchConfigForTest(0, true)
	got := svc.ManagedWebSearchAccount(context.Background(), &groupID, body)
	require.NotNil(t, got)
	require.Equal(t, capable.ID, got.ID)

	// The global emulation switch is not a master switch for the capability
	// path: an account marked web_search_capable is enough.
	setManagedWebSearchConfigForTest(0, false)
	got = svc.ManagedWebSearchAccount(context.Background(), &groupID, body)
	require.NotNil(t, got)
	require.Equal(t, capable.ID, got.ID)

	repo.groupAccount[1].Extra = nil
	require.Nil(t, svc.ManagedWebSearchAccount(context.Background(), &groupID, body))
}

func TestCountAnthropicWebSearchCalls(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"thinking","thinking":"..."},
			{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"a"}},
			{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[]},
			{"type":"server_tool_use","id":"srvtoolu_2","name":"web_search","input":{"query":"b"}},
			{"type":"server_tool_use","id":"srvtoolu_3","name":"other_tool","input":{}}
		]
	}`)
	require.Equal(t, 2, countAnthropicWebSearchCalls(gjson.GetBytes(body, "content")))
}

func TestAnthropicStreamEventIsWebSearchToolUse(t *testing.T) {
	require.True(t, anthropicStreamEventIsWebSearchToolUse(
		`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{}}}`,
	))
	require.False(t, anthropicStreamEventIsWebSearchToolUse(
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	))
	require.False(t, anthropicStreamEventIsWebSearchToolUse(
		`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","name":"other_tool"}}`,
	))
}

func TestWebSearchEmulationConfigManagedAccount(t *testing.T) {
	var nilCfg *WebSearchEmulationConfig
	require.Zero(t, nilCfg.ManagedAccount())

	zero := int64(0)
	require.Zero(t, (&WebSearchEmulationConfig{ManagedAccountID: &zero}).ManagedAccount())

	negative := int64(-1)
	require.Zero(t, (&WebSearchEmulationConfig{ManagedAccountID: &negative}).ManagedAccount())

	id := int64(918)
	require.Equal(t, int64(918), (&WebSearchEmulationConfig{ManagedAccountID: &id}).ManagedAccount())
}

func TestNativeAnthropicPassthroughCountsWebSearchCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"deepseek-v4-flash","max_tokens":32,"stream":false,` +
		`"messages":[{"role":"user","content":"search"}],` +
		`"tools":[{"type":"web_search_20250305","name":"web_search"}]}`)
	resp := nativeAnthropicBufferedResponse()
	resp.Body = io.NopCloser(strings.NewReader(
		`{"id":"msg_1","type":"message","role":"assistant","model":"deepseek-v4-flash",` +
			`"content":[` +
			`{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"x"}},` +
			`{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[]},` +
			`{"type":"text","text":"done"}],` +
			`"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`,
	))
	upstream := &httpUpstreamRecorder{resp: resp}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardAsAnthropic(context.Background(),
		adaptiveProtocolTestContext("/v1/messages", body), nativeAnthropicTestAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
}
