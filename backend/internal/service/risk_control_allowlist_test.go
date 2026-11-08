package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type failingAllowlistRepo struct {
	fakeSettingRepo
	failKey string
}

func (r *failingAllowlistRepo) GetValue(ctx context.Context, key string) (string, error) {
	if key == r.failKey || r.failKey == "all" {
		return "", errors.New("database unavailable")
	}
	return r.fakeSettingRepo.GetValue(ctx, key)
}

func TestRiskControlAllowlistRetainsLastGoodValueAndRetries(t *testing.T) {
	for _, failKey := range []string{SettingKeyCyberPolicyUserAllowlist, "all"} {
		t.Run(failKey, func(t *testing.T) {
			repo := &failingAllowlistRepo{fakeSettingRepo: fakeSettingRepo{vals: map[string]string{SettingKeyCyberPolicyUserAllowlist: "12"}}}
			svc := &SettingService{settingRepo: repo}
			ctx := context.Background()
			require.True(t, svc.IsCyberPolicyUserAllowlisted(ctx, 12))
			expire := func() {
				cached, ok := svc.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime)
				require.True(t, ok)
				old := *cached
				old.expiresAt = 0
				svc.cyberSessionBlockRuntimeCache.Store(&old)
			}
			expire()
			repo.failKey = failKey
			require.True(t, svc.IsCyberPolicyUserAllowlisted(ctx, 12))
			cached, ok := svc.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime)
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(time.Unix(0, cached.expiresAt)), cyberSessionBlockRuntimeErrorTTL)
			repo.failKey = ""
			repo.vals[SettingKeyCyberPolicyUserAllowlist] = ""
			expire()
			require.False(t, svc.IsCyberPolicyUserAllowlisted(ctx, 12), "successful removal must replace stale membership")
		})
	}
}

func TestRiskControlAllowlistAuditsWithoutLocalPenalties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{CategoryScores: map[string]float64{"sexual": 0.99}}}})
	}))
	defer server.Close()
	for _, path := range []string{"keyword", "hash", "pre_block", "observe"} {
		t.Run(path, func(t *testing.T) {
			cfg := defaultContentModerationConfig()
			cfg.Enabled, cfg.AutoBanEnabled, cfg.EmailOnHit = true, true, true
			cfg.BanThreshold = 1
			cfg.Mode = ContentModerationModePreBlock
			cfg.BaseURL, cfg.APIKeys = server.URL, []string{"sk-test"}
			if path == "observe" {
				cfg.Mode = ContentModerationModeObserve
			}
			if path == "keyword" {
				cfg.BlockedKeywords = []string{"blocked prompt"}
			}
			hashes := &contentModerationTestHashCache{hashes: map[string]struct{}{}}
			if path == "hash" {
				cfg.PreHashCheckEnabled = true
				hashes.hashes[(ContentModerationInput{Text: "blocked prompt"}).Hash()] = struct{}{}
			}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			repo := &banCountArgsTestRepo{}
			svc := &ContentModerationService{
				settingRepo: &contentModerationTestSettingRepo{values: map[string]string{
					SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: string(raw), SettingKeyCyberPolicyUserAllowlist: "12",
				}},
				repo: repo, hashCache: hashes, emailService: &EmailService{},
				httpClient: server.Client(), asyncQueue: make(chan contentModerationTask, 4), keyHealth: make(map[string]*contentModerationKeyHealth),
			}
			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
				UserID: 12, UserEmail: "trusted@example.com", Protocol: ContentModerationProtocolOpenAIChat,
				Body: []byte(`{"messages":[{"role":"user","content":"blocked prompt"}]}`),
			})
			require.NoError(t, err)
			require.True(t, decision.Allowed)
			require.False(t, decision.Blocked)
			var task contentModerationTask
			select {
			case task = <-svc.asyncQueue:
			case <-time.After(time.Second):
				t.Fatal("expected an audit task")
			}
			if task.log != nil {
				svc.persistContentModerationLog(context.Background(), task.config, task.log, task.inputHash, task.recordHash, task.applySideEffects)
			} else {
				delay := 1
				svc.checkSync(context.Background(), task.input, svc.runtimeSnapshot.Load().config, task.content, task.inputHash, &delay, false)
			}
			logs := repo.snapshotLogs()
			require.Len(t, logs, 1)
			require.True(t, logs[0].Flagged)
			require.Equal(t, ContentModerationModeRiskControlLogOnly, logs[0].Mode)
			require.False(t, logs[0].AutoBanned)
			require.False(t, logs[0].EmailSent)
			require.Zero(t, logs[0].ViolationCount)
			require.Empty(t, repo.snapshotCountCalls())
			require.Zero(t, svc.preBlockBlocked.Load())
			if path != "hash" {
				require.Empty(t, hashes.hashes)
			}
		})
	}
}

type failingModerationAllowlistRepo struct {
	contentModerationTestSettingRepo
	fail bool
}

func (r *failingModerationAllowlistRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if r.fail {
		return nil, errors.New("database unavailable")
	}
	return r.contentModerationTestSettingRepo.GetMultiple(ctx, keys)
}

func TestRiskControlAllowlistSnapshotRetainsMembershipOnFailure(t *testing.T) {
	repo := &failingModerationAllowlistRepo{contentModerationTestSettingRepo: contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled: "true", SettingKeyCyberPolicyUserAllowlist: "12",
	}}}
	svc := &ContentModerationService{settingRepo: repo}
	ctx := context.Background()
	_, err := svc.refreshRuntimeSnapshot(ctx)
	require.NoError(t, err)
	repo.fail = true
	_, err = svc.refreshRuntimeSnapshot(ctx)
	require.Error(t, err)
	require.Contains(t, svc.runtimeSnapshot.Load().allowlistedUsers, int64(12))
	repo.fail = false
	repo.values[SettingKeyCyberPolicyUserAllowlist] = "34"
	_, err = svc.refreshRuntimeSnapshot(ctx)
	require.NoError(t, err)
	require.NotContains(t, svc.runtimeSnapshot.Load().allowlistedUsers, int64(12))
	require.Contains(t, svc.runtimeSnapshot.Load().allowlistedUsers, int64(34))
}
