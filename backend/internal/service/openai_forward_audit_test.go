package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/forwardaudit"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestDoOpenAIUpstreamActivatesAuditOnlyForFinalOpenAIAccount(t *testing.T) {
	for _, test := range []struct {
		name        string
		platform    string
		wantRecords bool
	}{
		{name: "openai", platform: PlatformOpenAI, wantRecords: true},
		{name: "grok", platform: PlatformGrok, wantRecords: false},
		{name: "anthropic", platform: PlatformAnthropic, wantRecords: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
			t.Cleanup(recorder.Close)

			clientRequest, err := http.NewRequest(
				http.MethodPost,
				"https://sub2api.example/v1/responses",
				strings.NewReader(`{"model":"client-model"}`),
			)
			require.NoError(t, err)
			clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 42, 7, "session-final-account")
			_, err = io.Copy(io.Discard, clientRequest.Body)
			require.NoError(t, err)
			require.NoError(t, clientRequest.Body.Close())

			upstreamRequest, err := http.NewRequestWithContext(
				clientRequest.Context(),
				http.MethodPost,
				"https://upstream.example/v1/responses",
				strings.NewReader(`{"model":"final-model"}`),
			)
			require.NoError(t, err)

			upstream := &auditActivationHTTPUpstream{}
			gateway := &OpenAIGatewayService{httpUpstream: upstream}
			response, err := gateway.doOpenAIUpstream(upstreamRequest, "", &Account{
				ID: 99, Platform: test.platform, Type: AccountTypeAPIKey, Concurrency: 1,
			})
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			recorder.Close()
			records := readServiceForwardAuditRecords(t, directory)
			if test.wantRecords {
				require.Len(t, records, 3)
			} else {
				require.Empty(t, records)
			}
		})
	}
}

func TestAuditOpenAIPluginRoundTripCapturesRequestAndRawResponse(t *testing.T) {
	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	clientRequest, err := http.NewRequest(
		http.MethodPost,
		"https://sub2api.example/v1/responses",
		strings.NewReader(`{"model":"client-model"}`),
	)
	require.NoError(t, err)
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 42, 7, "session-plugin")
	_, err = io.Copy(io.Discard, clientRequest.Body)
	require.NoError(t, err)
	require.NoError(t, clientRequest.Body.Close())

	upstreamRequest, err := http.NewRequestWithContext(
		clientRequest.Context(),
		http.MethodPost,
		"https://api.openai.example/v1/responses",
		strings.NewReader(`{"model":"normalized-model"}`),
	)
	require.NoError(t, err)
	upstreamRequest = forwardaudit.ActivateRequest(upstreamRequest)

	response, err := auditOpenAIPluginRoundTrip(upstreamRequest, 99, func(request *http.Request) (*http.Response, error) {
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.JSONEq(t, `{"model":"normalized-model"}`, string(body))
		require.NoError(t, request.Body.Close())
		return &http.Response{
			Status:     "201 Created",
			StatusCode: http.StatusCreated,
			Header:     http.Header{"X-Plugin-Upstream": []string{"raw"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"plugin-response"}`)),
		}, nil
	})
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	recorder.Close()
	records := readServiceForwardAuditRecords(t, directory)
	require.Len(t, records, 3)
}

func TestDoOpenAIUpstreamClearsStaleAuditActivationForNonOpenAIRetry(t *testing.T) {
	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	clientRequest, err := http.NewRequest(
		http.MethodPost,
		"https://sub2api.example/v1/responses",
		strings.NewReader(`{"model":"client-model"}`),
	)
	require.NoError(t, err)
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 42, 7, "session-retry")
	_, err = io.Copy(io.Discard, clientRequest.Body)
	require.NoError(t, err)
	require.NoError(t, clientRequest.Body.Close())

	reusedRequest, err := http.NewRequestWithContext(
		clientRequest.Context(),
		http.MethodPost,
		"https://api.x.ai/v1/responses",
		strings.NewReader(`{"model":"grok-4"}`),
	)
	require.NoError(t, err)
	// Simulate a request/context reused after an earlier OpenAI attempt.
	reusedRequest = forwardaudit.ActivateRequest(reusedRequest)

	gateway := &OpenAIGatewayService{httpUpstream: &auditActivationHTTPUpstream{}}
	response, err := gateway.doOpenAIUpstream(reusedRequest, "", &Account{
		ID: 100, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
	})
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	recorder.Close()
	require.Empty(t, readServiceForwardAuditRecords(t, directory))
}

func TestFetchOpenAIModelsUpstreamActivatesAuditForSelectedOpenAIAccount(t *testing.T) {
	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	clientRequest, err := http.NewRequest(http.MethodGet, "https://sub2api.example/v1/models", nil)
	require.NoError(t, err)
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 42, 7, "session-models")

	gateway := &OpenAIGatewayService{httpUpstream: &auditActivationHTTPUpstream{}}
	response, err := gateway.fetchOpenAIModelsUpstream(clientRequest.Context(), openAIModelsRequest{
		url:                "https://api.openai.example/v1/models",
		headers:            make(http.Header),
		accountID:          99,
		accountConcurrency: 1,
		useAPIKeyUpstream:  true,
		credentialAccount: &Account{
			ID: 99, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		},
	}, "")
	require.NoError(t, err)
	require.NotNil(t, response)

	recorder.Close()
	require.Len(t, readServiceForwardAuditRecords(t, directory), 3)
}

func TestFetchOpenAIModelsListColdRefreshPreservesAuditTraceAttribution(t *testing.T) {
	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	gateway := newCodexModelsAPIKeyTestService(&auditModelsHTTPUpstream{roundTrip: func(_ *http.Request) (*http.Response, error) {
		return auditModelsResponse(`{"object":"list","data":[{"id":"gpt-audit"}]}`), nil
	}})
	clientRequest, err := http.NewRequest(http.MethodGet, "https://sub2api.example/v1/models", nil)
	require.NoError(t, err)
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 42, 7, "session-models-cold")

	response, err := gateway.FetchOpenAIModelsList(
		clientRequest.Context(),
		newCodexModelsAPIKeyTestAccount("https://models.example/v1"),
	)
	require.NoError(t, err)
	require.NotNil(t, response)

	recorder.Close()
	requireServiceAuditAttribution(t, readServiceForwardAuditRecords(t, directory), 42, 7, "session-models-cold")
}

func TestFetchOpenAIModelsListBackgroundRefreshPreservesAuditTraceAttribution(t *testing.T) {
	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	var calls atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	refreshFinished := make(chan struct{})
	gateway := newCodexModelsAPIKeyTestService(&auditModelsHTTPUpstream{roundTrip: func(_ *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return auditModelsResponse(`{"object":"list","data":[{"id":"old"}]}`), nil
		}
		close(refreshStarted)
		<-releaseRefresh
		response := auditModelsResponse(`{"object":"list","data":[{"id":"new"}]}`)
		response.Body = &auditModelsSignalingBody{
			Reader: strings.NewReader(`{"object":"list","data":[{"id":"new"}]}`),
			done:   refreshFinished,
		}
		return response, nil
	}})
	account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	_, err := gateway.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)

	gateway.openAIModelsCache.mu.Lock()
	for key, entry := range gateway.openAIModelsCache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		gateway.openAIModelsCache.entries[key] = entry
	}
	gateway.openAIModelsCache.mu.Unlock()

	requestContext, cancelRequest := context.WithCancel(context.Background())
	clientRequest, err := http.NewRequestWithContext(requestContext, http.MethodGet, "https://sub2api.example/v1/models", nil)
	require.NoError(t, err)
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 84, 17, "session-models-background")

	stale, err := gateway.FetchOpenAIModelsList(clientRequest.Context(), account)
	require.NoError(t, err)
	require.Contains(t, string(stale.Body), `"old"`)
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background models refresh did not start")
	}

	cancelRequest()
	forwardaudit.ReleaseRequest(clientRequest)
	close(releaseRefresh)
	select {
	case <-refreshFinished:
	case <-time.After(time.Second):
		t.Fatal("background models refresh did not finish after caller cancellation")
	}

	recorder.Close()
	requireServiceAuditAttribution(t, readServiceForwardAuditRecords(t, directory), 84, 17, "session-models-background")
}

func TestFetchOpenAIModelsUpstreamDoesNotAuditNonOpenAICredentialAccount(t *testing.T) {
	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	clientRequest, err := http.NewRequest(http.MethodGet, "https://sub2api.example/v1/models", nil)
	require.NoError(t, err)
	clientRequest = forwardaudit.AttachRequest(clientRequest, recorder, 42, 7, "session-models-grok")
	gateway := newCodexModelsAPIKeyTestService(&auditModelsHTTPUpstream{roundTrip: func(_ *http.Request) (*http.Response, error) {
		return auditModelsResponse(`{"object":"list","data":[{"id":"grok"}]}`), nil
	}})

	response, err := gateway.fetchOpenAIModelsUpstream(clientRequest.Context(), openAIModelsRequest{
		url:                "https://api.x.ai/v1/models",
		headers:            make(http.Header),
		accountID:          99,
		accountConcurrency: 1,
		useAPIKeyUpstream:  true,
		credentialAccount: &Account{
			ID: 99, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1,
		},
	}, "")
	require.NoError(t, err)
	require.NotNil(t, response)

	recorder.Close()
	require.Empty(t, readServiceForwardAuditRecords(t, directory))
}

type auditModelsHTTPUpstream struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (u *auditModelsHTTPUpstream) Do(request *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	return forwardaudit.WrapTransport(serviceAuditRoundTripFunc(u.roundTrip), accountID).RoundTrip(request)
}

func (u *auditModelsHTTPUpstream) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(request, proxyURL, accountID, accountConcurrency)
}

type auditModelsSignalingBody struct {
	*strings.Reader
	done chan<- struct{}
	once sync.Once
}

func (b *auditModelsSignalingBody) Close() error {
	b.once.Do(func() { close(b.done) })
	return nil
}

func auditModelsResponse(body string) *http.Response {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func requireServiceAuditAttribution(t *testing.T, records []map[string]any, userID, apiKeyID int64, sessionID string) {
	t.Helper()
	require.Len(t, records, 3)
	stages := make([]string, 0, len(records))
	for _, record := range records {
		require.EqualValues(t, userID, record["user_id"])
		require.EqualValues(t, apiKeyID, record["api_key_id"])
		require.Equal(t, sessionID, record["session_id"])
		stages = append(stages, record["stage"].(string))
	}
	require.ElementsMatch(t, []string{"client_request", "upstream_request", "upstream_response"}, stages)
}

type auditActivationHTTPUpstream struct{}

func (u *auditActivationHTTPUpstream) Do(request *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	return forwardaudit.WrapTransport(serviceAuditRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Body != nil {
			_, _ = io.Copy(io.Discard, request.Body)
			_ = request.Body.Close()
		}
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"response"}`)),
		}, nil
	}), accountID).RoundTrip(request)
}

func (u *auditActivationHTTPUpstream) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(request, proxyURL, accountID, accountConcurrency)
}

type serviceAuditRoundTripFunc func(*http.Request) (*http.Response, error)

func (f serviceAuditRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func readServiceForwardAuditRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	var records []map[string]any
	require.NoError(t, filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal(line, &record); err != nil {
				return err
			}
			records = append(records, record)
		}
		return nil
	}))
	return records
}
