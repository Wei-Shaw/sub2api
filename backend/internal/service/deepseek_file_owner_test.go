//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type testDeepseekFileCache struct {
	schedulerTestGatewayCache
	files            map[int64]map[string]DeepseekFile
	getErr, writeErr error
	onSave           func()
}

func (c *testDeepseekFileCache) SaveDeepseekFile(_ context.Context, userID int64, f DeepseekFile) error {
	if c.onSave != nil {
		c.onSave()
	}
	if c.writeErr != nil {
		return c.writeErr
	}
	if c.files == nil {
		c.files = make(map[int64]map[string]DeepseekFile)
	}
	if c.files[userID] == nil {
		c.files[userID] = make(map[string]DeepseekFile)
	}
	c.files[userID][f.ID] = f
	return nil
}

func (c *testDeepseekFileCache) DeepseekFileAccounts(_ context.Context, userID int64, ids []string) (map[string]int64, error) {
	accounts := make(map[string]int64)
	for _, id := range ids {
		accounts[id] = c.files[userID][id].AccountID
	}
	return accounts, c.getErr
}

func (c *testDeepseekFileCache) ListDeepseekFiles(_ context.Context, userID int64) ([]DeepseekFile, error) {
	var files []DeepseekFile
	for _, file := range c.files[userID] {
		files = append(files, file)
	}
	return files, c.getErr
}

func (c *testDeepseekFileCache) DeleteDeepseekFile(_ context.Context, userID, accountID int64, id string) error {
	if c.files[userID][id].AccountID == accountID {
		delete(c.files[userID], id)
	}
	return c.writeErr
}

func TestAnthropicFileIDs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "direct image file source",
			body: `{"messages":[{"role":"user","content":[
				{"type":"text","text":"look"},
				{"type":"image","source":{"type":"file","file_id":"file_a"}}
			]}]}`,
			want: []string{"file_a"},
		},
		{
			name: "tool_result nested image file source",
			body: `{"messages":[{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_1","content":[
					{"type":"image","source":{"type":"file","file_id":"file_b"}}
				]}
			]}]}`,
			want: []string{"file_b"},
		},
		{
			name: "document file source",
			body: `{"messages":[{"role":"user","content":[
				{"type":"document","source":{"type":"file","file_id":"file_c"}}
			]}]}`,
			want: []string{"file_c"},
		},
		{
			name: "base64 image source is ignored",
			body: `{"messages":[{"role":"user","content":[
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
			]}]}`,
			want: nil,
		},
		{
			name: "text only content",
			body: `{"messages":[{"role":"user","content":"plain text"}]}`,
			want: nil,
		},
		{
			name: "mixed blocks dedupe preserving order",
			body: `{"messages":[
				{"role":"user","content":[
					{"type":"image","source":{"type":"file","file_id":"file_b"}},
					{"type":"document","source":{"type":"file","file_id":"file_a"}}
				]},
				{"role":"assistant","content":[
					{"type":"tool_result","tool_use_id":"toolu_2","content":[
						{"type":"image","source":{"type":"file","file_id":"file_b"}},
						{"type":"image","source":{"type":"base64","data":"x"}}
					]}
				]}
			]}`,
			want: []string{"file_b", "file_a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, deepseekFileIDs([]byte(tt.body), APIProtocolAnthropic))
		})
	}
}

func TestDeepseekFileReferencesPinEachProtocol(t *testing.T) {
	ctx := context.Background()
	groupID := int64(163)
	owner := Account{ID: 163, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive,
		Schedulable: true, Concurrency: 50, Priority: 99, GroupIDs: []int64{groupID}}
	other := owner
	other.ID, other.Priority = 167, 1
	cache := &testDeepseekFileCache{}
	require.NoError(t, cache.SaveDeepseekFile(ctx, 1, DeepseekFile{ID: "f", AccountID: owner.ID}))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, cache: cache,
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{owner, other}}}
	selectAccount := func(ctx context.Context, failed map[int64]struct{}) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
		return svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "", "deepseek-flash", failed,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, PlatformDeepseek)
	}
	base, _, err := selectAccount(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, other.ID, base.Account.ID)
	base.ReleaseFunc()
	for _, tt := range []struct{ protocol, body string }{
		{APIProtocolAnthropic, `{"messages":[{"content":[{"type":"document","source":{"type":"file","file_id":"f"}}]}]}`},
		{APIProtocolChatCompletions, `{"messages":[{"content":[{"type":"file","file_id":"f"}]}]}`},
		{APIProtocolChatCompletions, `{"messages":[{"content":[{"type":"file","file":{"file_id":"f"}}]}]}`},
		{APIProtocolResponses, `{"input":[{"content":[{"type":"input_file","file_id":"f"}]}]}`},
		{APIProtocolResponses, `{"input":[{"type":"function_call_output","output":[{"type":"input_image","file_id":"f"}]}]}`},
	} {
		t.Run(tt.protocol, func(t *testing.T) {
			pinned, err := svc.WithDeepseekFileReferences(ctx, PlatformDeepseek, tt.protocol, 1, []byte(tt.body))
			require.NoError(t, err)
			got, decision, err := selectAccount(pinned, nil)
			require.NoError(t, err)
			require.Equal(t, owner.ID, got.Account.ID)
			require.True(t, decision.StickySessionHit)
			got.ReleaseFunc()
			_, _, err = selectAccount(pinned, map[int64]struct{}{owner.ID: {}})
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			_, err = svc.WithDeepseekFileReferences(ctx, PlatformDeepseek, tt.protocol, 2, []byte(tt.body))
			require.Equal(t, 400, infraerrors.Code(err))
		})
	}
}

func TestDeepseekFileReferencesMixedOwnersAndStoreFailure(t *testing.T) {
	ctx := context.Background()
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cache: cache}
	for _, f := range []DeepseekFile{{ID: "a", AccountID: 1}, {ID: "b", AccountID: 2}} {
		require.NoError(t, cache.SaveDeepseekFile(ctx, 1, f))
	}
	for _, tc := range []struct {
		ids    []string
		status int
	}{
		{[]string{"a", "b"}, 400}, {[]string{"a", "missing"}, 404},
	} {
		id, err := svc.DeepseekFileAccount(ctx, 1, tc.ids)
		require.Equal(t, tc.status, infraerrors.Code(err))
		require.Zero(t, id)
	}
	cache.getErr = errors.New("redis unavailable")
	for _, cache := range []GatewayCache{cache, &schedulerTestGatewayCache{}, nil} {
		svc.cache = cache
		_, err := svc.DeepseekFileAccount(ctx, 1, []string{"a"})
		require.Equal(t, 503, infraerrors.Code(err))
		_, err = svc.WithDeepseekFileReferences(ctx, PlatformDeepseek, APIProtocolChatCompletions, 1,
			[]byte(`{"messages":[{"content":[{"type":"file","file_data":"inline"}]}]}`))
		require.NoError(t, err)
	}
}

func TestDeepseekFileOwnerUnavailable(t *testing.T) {
	for _, state := range []string{"available", "unavailable", "missing", "wrong platform", "other group"} {
		t.Run(state, func(t *testing.T) {
			groupID := int64(24)
			ownerID := int64(1)
			owner := Account{ID: ownerID, Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 50, GroupIDs: []int64{groupID}}
			other := owner
			other.ID = 2
			if state == "unavailable" {
				until := time.Now().Add(time.Minute)
				owner.TempUnschedulableUntil = &until
			}
			if state == "wrong platform" {
				owner.Platform = PlatformOpenAI
			}
			accounts := []Account{owner, other}
			if state == "missing" {
				accounts = []Account{other}
			}
			if state == "other group" {
				owner.GroupIDs = []int64{1234}
				accounts[0] = owner
			}
			var acquired, released []int64
			cfg := &config.Config{}
			cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
			cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
			cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache:       &schedulerTestGatewayCache{},
				cfg:         cfg,
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					acquireResults: map[int64]bool{1: true, 2: true},
					acquiredIDs:    &acquired,
					releasedIDs:    &released,
				}),
			}
			ctx := context.Background()
			selection, decision, err := svc.SelectAccountWithSchedulerForCapability(withDeepseekFileAccount(ctx, ownerID), &groupID, "", "", "deepseek-flash", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, PlatformDeepseek)
			switch state {
			case "available":
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(1), selection.Account.ID)
				require.True(t, selection.Acquired)
				require.True(t, decision.StickySessionHit)
				selection.ReleaseFunc()
				require.Equal(t, []int64{1}, released)
			default:
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Nil(t, selection)
			}
			require.NotContains(t, acquired, int64(2), "file owner pin must never fall back to another account")
		})
	}
}
