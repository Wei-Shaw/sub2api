package securityaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type promptAuditHashCacheFake struct {
	hashes   map[string]struct{}
	checked  []string
	recorded []string
	hasErr   error
}

func (c *promptAuditHashCacheFake) RecordFlaggedPromptHash(_ context.Context, promptHash string) error {
	if c.hashes == nil {
		c.hashes = map[string]struct{}{}
	}
	c.recorded = append(c.recorded, promptHash)
	c.hashes[promptHash] = struct{}{}
	return nil
}

func (c *promptAuditHashCacheFake) HasFlaggedPromptHash(_ context.Context, promptHash string) (bool, error) {
	c.checked = append(c.checked, promptHash)
	if c.hasErr != nil {
		return false, c.hasErr
	}
	_, ok := c.hashes[promptHash]
	return ok, nil
}

func (c *promptAuditHashCacheFake) DeleteFlaggedPromptHash(_ context.Context, promptHash string) (bool, error) {
	if _, ok := c.hashes[promptHash]; !ok {
		return false, nil
	}
	delete(c.hashes, promptHash)
	return true, nil
}

func (c *promptAuditHashCacheFake) ClearFlaggedPromptHashes(context.Context) (int64, error) {
	count := int64(len(c.hashes))
	c.hashes = map[string]struct{}{}
	return count, nil
}

func (c *promptAuditHashCacheFake) CountFlaggedPromptHashes(context.Context) (int64, error) {
	return int64(len(c.hashes)), nil
}

func promptHashTestRequest() Request {
	return Request{
		RequestID: "hash-request",
		Protocol:  "openai_chat_completions",
		Body:      []byte(`{"messages":[{"role":"system","content":"keep this instruction"},{"role":"user","content":"same prompt"}]}`),
	}
}

func TestPromptServicePreCheckBlocksPreviouslyFlaggedPromptWithoutScanner(t *testing.T) {
	req := promptHashTestRequest()
	snapshot, err := ExtractBlockingPromptSnapshot(req)
	require.NoError(t, err)
	cache := &promptAuditHashCacheFake{hashes: map[string]struct{}{snapshot.PromptHash: {}}}
	metrics := NewAtomicMetrics()
	service := &PromptService{
		config: &fakeConfigStore{active: true, cfg: ActiveConfig{
			RiskControlEnabled: true, Enabled: true, PreHashCheckEnabled: true, AllGroups: true, ConfigVersion: 7,
		}},
		hashCache: cache,
		metrics:   metrics,
	}

	decision, err := service.PreCheck(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, DecisionBlock, decision.Kind)
	require.Equal(t, ErrorCodeBlocked, decision.ErrorCode)
	require.Equal(t, "hash", decision.Result.ScannerBackend)
	require.Equal(t, []string{snapshot.PromptHash}, cache.checked)
	require.Equal(t, int64(1), metrics.Snapshot().Blocked)
}

func TestPromptServicePreCheckFailsOpenWhenCacheUnavailableOrDisabled(t *testing.T) {
	req := promptHashTestRequest()
	for _, test := range []struct {
		name  string
		cache PromptAuditHashCache
		cfg   ActiveConfig
	}{
		{name: "disabled", cache: &promptAuditHashCacheFake{}, cfg: ActiveConfig{RiskControlEnabled: true, Enabled: true}},
		{name: "cache error", cache: &promptAuditHashCacheFake{hasErr: errors.New("redis down")}, cfg: ActiveConfig{RiskControlEnabled: true, Enabled: true, PreHashCheckEnabled: true, AllGroups: true}},
		{name: "no cache", cache: nil, cfg: ActiveConfig{RiskControlEnabled: true, Enabled: true, PreHashCheckEnabled: true, AllGroups: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &PromptService{config: &fakeConfigStore{active: true, cfg: test.cfg}, hashCache: test.cache}
			decision, err := service.PreCheck(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, DecisionAllow, decision.Kind)
		})
	}
}

func TestRunnerRecordsBlockedAsyncPromptHashAfterEventCompletion(t *testing.T) {
	cache := &promptAuditHashCacheFake{}
	cfg := asyncConfig()
	cfg.PreHashCheckEnabled = true
	repo := &fakeJobRepository{}
	payload := &fakePayloadStore{values: map[int64]string{51: "abcdef"}}
	runner := NewRunnerWithHashCache(
		&fakeConfigStore{cfg: cfg, active: true}, repo, payload,
		PromptScannerFunc(func(_ context.Context, endpoint ActiveEndpoint, _ string, _ []string) (*NormalizedResult, error) {
			return &NormalizedResult{
				Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe",
				Categories: []string{"jailbreak"}, MatchedScanners: []string{"jailbreak"},
				ScannerScores: map[string]float64{"jailbreak": 1}, ScannerEvidence: map[string]string{"jailbreak": "block"},
				GuardEndpointID: endpoint.ID,
			}, nil
		}),
		NewAtomicMetrics(), NewPromptResultCache(), cache,
	)
	runner.clock = fixedClock{now: time.Unix(100, 0).UTC()}
	job := workerJob(1, 3)
	job.Snapshot.PromptHash = strings.Repeat("a", 64)

	require.NoError(t, runner.processJob(context.Background(), 0, cfg, job))
	require.Equal(t, []string{job.Snapshot.PromptHash}, cache.recorded)
}
