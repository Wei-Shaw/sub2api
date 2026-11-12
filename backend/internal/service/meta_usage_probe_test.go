//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type metaProbeUpstream struct {
	status   int
	body     string
	requests []*http.Request
	payloads [][]byte
}

func (u *metaProbeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.requests = append(u.requests, req)
	payload, _ := io.ReadAll(req.Body)
	u.payloads = append(u.payloads, payload)
	return &http.Response{StatusCode: u.status, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}

func (u *metaProbeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

type fakeMetaUsageProber struct {
	calls   int
	updates map[string]any
	err     error
}

func (p *fakeMetaUsageProber) ProbeMetaSubscriptionUsage(_ context.Context, _ *Account) (map[string]any, error) {
	p.calls++
	return p.updates, p.err
}

func newMetaUsageProbeTestService(prober metaUsageProber) *AccountUsageService {
	return &AccountUsageService{
		accountRepo:     &metaAccountRepoStub{account: metaUsageTestAccount()},
		usageLogRepo:    &usageBatchLogRepoStub{},
		cache:           NewUsageCache(),
		metaUsageProber: prober,
	}
}

func TestProbeMetaSubscriptionUsageReadsWindowsFromStream(t *testing.T) {
	resetAt := time.Now().Add(2 * time.Hour).Unix()
	upstream := &metaProbeUpstream{status: http.StatusOK, body: metaUsageSSE(resetAt)}
	repo := &metaUsageRecordingRepo{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: repo, httpUpstream: upstream}

	updates, err := svc.ProbeMetaSubscriptionUsage(context.Background(), metaUsageTestAccount())
	require.NoError(t, err)
	require.InDelta(t, 0.42, updates[metaUsageWindowUtilizationKey], 0.0001)
	require.Equal(t, resetAt, updates[metaUsageWindowResetKey])
	require.Equal(t, "muse-pro", updates[metaUsageTierKey])
	require.NotEmpty(t, updates[metaUsageSampledAtKey])
	require.Len(t, repo.updates, 1)
	require.Equal(t, updates, repo.updates[0])

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://api.meta.ai/v1/responses", req.URL.String())
	require.Equal(t, "Bearer sk-muse", req.Header.Get("Authorization"))
	require.Equal(t, "tbh:exec", req.Header.Get("X-Client-Id"))
	require.True(t, strings.HasPrefix(req.Header.Get("User-Agent"), "muse-build/"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(upstream.payloads[0], &payload))
	require.Equal(t, true, payload["stream"])
	require.Equal(t, DefaultMetaTestModel, payload["model"])
	require.EqualValues(t, metaUsageProbeMaxOutput, payload["max_output_tokens"])
	require.NotContains(t, payload, "tools")
}

func TestProbeMetaSubscriptionUsageReportsUpstreamErrors(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: &metaUsageRecordingRepo{}}

	svc.httpUpstream = &metaProbeUpstream{status: http.StatusTooManyRequests, body: `{"error":{"message":"Subscription quota exhausted.","resets_at":1}}`}
	_, err := svc.ProbeMetaSubscriptionUsage(context.Background(), metaUsageTestAccount())
	require.ErrorContains(t, err, "429")
	require.ErrorContains(t, err, "quota exhausted")

	svc.httpUpstream = &metaProbeUpstream{status: http.StatusOK, body: "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n"}
	_, err = svc.ProbeMetaSubscriptionUsage(context.Background(), metaUsageTestAccount())
	require.ErrorIs(t, err, errMetaUsageNotReported)
}

func TestAccountUsageServiceMetaQueryProbesOnlyWhenForced(t *testing.T) {
	resetAt := time.Now().Add(90 * time.Minute).Unix()
	prober := &fakeMetaUsageProber{updates: map[string]any{
		metaUsageWindowUtilizationKey: 0.37,
		metaUsageWindowResetKey:       resetAt,
		metaUsageTierKey:              "muse-pro",
	}}
	svc := newMetaUsageProbeTestService(prober)

	passive, err := svc.getUsageForAccount(context.Background(), metaUsageTestAccount(), false)
	require.NoError(t, err)
	require.Zero(t, prober.calls)
	require.Zero(t, passive.FiveHour.Utilization)

	account := metaUsageTestAccount()
	usage, err := svc.getUsageForAccount(context.Background(), account, true)
	require.NoError(t, err)
	require.Equal(t, 1, prober.calls)
	require.InDelta(t, 37.0, usage.FiveHour.Utilization, 0.001)
	require.NotNil(t, usage.FiveHour.ResetsAt)
	require.Equal(t, "muse-pro", usage.SubscriptionTierID)
	require.Empty(t, usage.SubscriptionTierRaw)
	require.Equal(t, "active", usage.Source)
	require.Equal(t, "passive", passive.Source)
	require.Empty(t, usage.Error)
	require.Equal(t, 0.37, account.Extra[metaUsageWindowUtilizationKey])
}

func TestAccountUsageServiceMetaQuerySurfacesProbeError(t *testing.T) {
	prober := &fakeMetaUsageProber{err: errors.New("meta usage probe returned 429: quota")}
	svc := newMetaUsageProbeTestService(prober)

	usage, err := svc.getUsageForAccount(context.Background(), metaUsageTestAccount(), true)
	require.NoError(t, err)
	require.Equal(t, 1, prober.calls)
	require.Contains(t, usage.Error, "429")
}

func TestAccountUsageServiceMetaQuerySkipsProbeForRestrictedAdmin(t *testing.T) {
	prober := &fakeMetaUsageProber{}
	svc := newMetaUsageProbeTestService(prober)
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{
		Permissions:      []string{"accounts.usage.read"},
		AllGroups:        true,
		IncludeUngrouped: true,
		AccountTypes:     []string{AccountTypeOAuth},
	}}}})

	_, err := svc.getUsageForAccount(ctx, metaUsageTestAccount(), true)
	require.NoError(t, err)
	require.Zero(t, prober.calls)
}
