package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type groupUpstreamModelsAccountRepoStub struct {
	AccountRepository
	accounts []Account
	err      error
}

func (r *groupUpstreamModelsAccountRepoStub) ListSchedulableByGroupID(_ context.Context, _ int64) ([]Account, error) {
	return r.accounts, r.err
}

func upstreamModelsResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func groupUpstreamModelsTestAccount(id int64, baseURL string) Account {
	return Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": baseURL,
		},
	}
}

type parallelGroupModelsResponse struct {
	status int
	body   string
}

type parallelGroupModelsUpstream struct {
	mu        sync.Mutex
	requests  []*http.Request
	responses map[string]parallelGroupModelsResponse
	started   chan struct{}
	release   chan struct{}
}

func (u *parallelGroupModelsUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req)
	response, ok := u.responses[req.URL.Hostname()]
	u.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no response configured for %s", req.URL.Hostname())
	}

	u.started <- struct{}{}
	select {
	case <-u.release:
		return upstreamModelsResponse(response.status, response.body), nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

func (u *parallelGroupModelsUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *parallelGroupModelsUpstream) requestSnapshot() []*http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]*http.Request(nil), u.requests...)
}

func TestFetchGroupUpstreamModelsMergesLiveModelsInParallel(t *testing.T) {
	upstream := &parallelGroupModelsUpstream{
		responses: map[string]parallelGroupModelsResponse{
			"upstream-one.example":   {status: http.StatusOK, body: `{"data":[{"id":"shared-model"},{"id":"alpha-model"}]}`},
			"upstream-two.example":   {status: http.StatusBadGateway, body: `{"error":{"message":"temporary upstream failure"}}`},
			"upstream-three.example": {status: http.StatusOK, body: `{"data":[{"id":"shared-model"},{"id":"beta-model"}]}`},
		},
		started: make(chan struct{}, 3),
		release: make(chan struct{}),
	}
	svc := &AccountTestService{
		accountRepo: &groupUpstreamModelsAccountRepoStub{accounts: []Account{
			groupUpstreamModelsTestAccount(1, "https://upstream-one.example"),
			groupUpstreamModelsTestAccount(2, "https://upstream-two.example"),
			groupUpstreamModelsTestAccount(3, "https://upstream-three.example"),
		}},
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	resultCh := make(chan struct {
		models []string
		err    error
	}, 1)
	go func() {
		models, err := svc.FetchGroupUpstreamModels(context.Background(), 42, PlatformOpenAI)
		resultCh <- struct {
			models []string
			err    error
		}{models: models, err: err}
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-upstream.started:
		case <-time.After(time.Second):
			close(upstream.release)
			t.Fatalf("expected all upstream probes to start in parallel; only %d started", i)
		}
	}
	close(upstream.release)
	result := <-resultCh

	require.NoError(t, result.err)
	require.Equal(t, []string{"alpha-model", "beta-model", "shared-model"}, result.models)
	requests := upstream.requestSnapshot()
	require.Len(t, requests, 3)
	require.ElementsMatch(t, []string{
		"https://upstream-one.example/v1/models",
		"https://upstream-two.example/v1/models",
		"https://upstream-three.example/v1/models",
	}, []string{requests[0].URL.String(), requests[1].URL.String(), requests[2].URL.String()})
}

func TestFetchGroupUpstreamModelsReturnsEmptyWhenNoAccountMatchesPlatform(t *testing.T) {
	svc := &AccountTestService{
		accountRepo: &groupUpstreamModelsAccountRepoStub{accounts: []Account{
			groupUpstreamModelsTestAccount(1, "https://upstream-one.example"),
		}},
		cfg: upstreamModelSyncTestConfig(),
	}

	models, err := svc.FetchGroupUpstreamModels(context.Background(), 42, PlatformGemini)

	require.NoError(t, err)
	require.Empty(t, models)
}

func TestFetchGroupUpstreamModelsFallsBackToModelMappingWhenListEndpointUnsupported(t *testing.T) {
	upstream := &parallelGroupModelsUpstream{
		responses: map[string]parallelGroupModelsResponse{
			"upstream-live.example": {
				status: http.StatusOK,
				body:   `{"data":[{"id":"live-model"},{"id":"MiniMax-M2.5"}]}`,
			},
			"upstream-no-list.example": {
				status: http.StatusNotFound,
				body:   `{"error":{"message":"not found"}}`,
			},
		},
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	mappedAccount := groupUpstreamModelsTestAccount(2, "https://upstream-no-list.example")
	mappedAccount.Credentials["model_mapping"] = map[string]any{
		"public-minimax": "minimax-m2.5",
		"public-glm":     "glm-5.2",
		"wildcard":       " Ignored-* ",
	}
	svc := &AccountTestService{
		accountRepo: &groupUpstreamModelsAccountRepoStub{accounts: []Account{
			groupUpstreamModelsTestAccount(1, "https://upstream-live.example"),
			mappedAccount,
		}},
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	resultCh := make(chan struct {
		models []string
		err    error
	}, 1)
	go func() {
		models, err := svc.FetchGroupUpstreamModels(context.Background(), 42, PlatformOpenAI)
		resultCh <- struct {
			models []string
			err    error
		}{models: models, err: err}
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-upstream.started:
		case <-time.After(time.Second):
			close(upstream.release)
			t.Fatalf("expected both upstream probes to start in parallel; only %d started", i)
		}
	}
	close(upstream.release)
	result := <-resultCh

	require.NoError(t, result.err)
	require.Equal(t, []string{"MiniMax-M2.5", "glm-5.2", "live-model", "minimax-m2.5"}, result.models)
}

func TestFetchGroupUpstreamModelsDoesNotFallbackOnTransientUpstreamErrors(t *testing.T) {
	upstream := &parallelGroupModelsUpstream{
		responses: map[string]parallelGroupModelsResponse{
			"upstream-live.example": {
				status: http.StatusOK,
				body:   `{"data":[{"id":"live-model"}]}`,
			},
			"upstream-fail.example": {
				status: http.StatusBadGateway,
				body:   `{"error":{"message":"temporary upstream failure"}}`,
			},
		},
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	mappedAccount := groupUpstreamModelsTestAccount(2, "https://upstream-fail.example")
	mappedAccount.Credentials["model_mapping"] = map[string]any{
		"public-minimax": "minimax-m2.5",
	}
	svc := &AccountTestService{
		accountRepo: &groupUpstreamModelsAccountRepoStub{accounts: []Account{
			groupUpstreamModelsTestAccount(1, "https://upstream-live.example"),
			mappedAccount,
		}},
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
	}

	resultCh := make(chan struct {
		models []string
		err    error
	}, 1)
	go func() {
		models, err := svc.FetchGroupUpstreamModels(context.Background(), 42, PlatformOpenAI)
		resultCh <- struct {
			models []string
			err    error
		}{models: models, err: err}
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-upstream.started:
		case <-time.After(time.Second):
			close(upstream.release)
			t.Fatalf("expected both upstream probes to start in parallel; only %d started", i)
		}
	}
	close(upstream.release)
	result := <-resultCh

	require.NoError(t, result.err)
	// 502 is not treated as an unsupported list endpoint, so mapping must not leak in.
	require.Equal(t, []string{"live-model"}, result.models)
}
