//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokImageCreateRepoStub struct {
	mu           sync.Mutex
	creates      map[string]GrokMediaImageCreateRecord
	cleanupErr   error
	cleanupCalls int
}

func grokImageCreateRepoKey(record GrokMediaImageCreateRecord) string {
	return fmt.Sprintf("%d:%d:%d:%s:%s", record.GroupID, record.UserID, record.APIKeyID, record.Endpoint, record.IdempotencyKeyHash)
}

func (r *grokImageCreateRepoStub) ClaimImageCreate(_ context.Context, record GrokMediaImageCreateRecord) (*GrokMediaImageCreateRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.creates == nil {
		r.creates = make(map[string]GrokMediaImageCreateRecord)
	}
	key := grokImageCreateRepoKey(record)
	if existing, ok := r.creates[key]; ok {
		if existing.RequestHash != record.RequestHash || existing.UpstreamIdempotencyKey != record.UpstreamIdempotencyKey {
			return nil, ErrGrokMediaImageIdempotencyConflict
		}
		copy := existing
		copy.ResponseBody = append([]byte(nil), existing.ResponseBody...)
		return &copy, nil
	}
	r.creates[key] = record
	copy := record
	return &copy, nil
}

func (r *grokImageCreateRepoStub) BindImageCreateAccount(_ context.Context, record GrokMediaImageCreateRecord, accountID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := grokImageCreateRepoKey(record)
	existing, ok := r.creates[key]
	if !ok {
		return 0, ErrGrokMediaImageIdempotencyUnavailable
	}
	if existing.AccountID == 0 {
		existing.AccountID = accountID
		r.creates[key] = existing
	}
	return existing.AccountID, nil
}

func (r *grokImageCreateRepoStub) ReleaseImageCreateAccount(_ context.Context, record GrokMediaImageCreateRecord, accountID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := grokImageCreateRepoKey(record)
	existing, ok := r.creates[key]
	if !ok || existing.AccountID != accountID {
		return false, nil
	}
	existing.AccountID = 0
	r.creates[key] = existing
	return true, nil
}

func (r *grokImageCreateRepoStub) CompleteImageCreate(_ context.Context, record GrokMediaImageCreateRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creates[grokImageCreateRepoKey(record)] = record
	return nil
}

func (r *grokImageCreateRepoStub) DeleteExpired(_ context.Context, before time.Time, limit int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupCalls++
	if r.cleanupErr != nil {
		return 0, r.cleanupErr
	}
	var deleted int64
	for key, record := range r.creates {
		if deleted >= int64(limit) {
			break
		}
		if !record.ExpiresAt.After(before) {
			delete(r.creates, key)
			deleted++
		}
	}
	return deleted, nil
}

type grokImageIdempotentUpstream struct {
	mu            sync.Mutex
	requests      int
	uniqueCreates int
	byKey         map[string]string
}

func (u *grokImageIdempotentUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests++
	key := req.Header.Get("Idempotency-Key")
	imageURL, ok := u.byKey[key]
	if !ok {
		u.uniqueCreates++
		imageURL = fmt.Sprintf("https://images.test/%d.png", u.uniqueCreates)
		u.byKey[key] = imageURL
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"created":1,"data":[{"url":%q}]}`, imageURL))),
	}, nil
}

func (u *grokImageIdempotentUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestCanonicalGrokMediaImageRequestHashJSONAndMultipart(t *testing.T) {
	jsonA := []byte(`{"model":"grok-imagine","prompt":"harbor","n":1}`)
	jsonB := []byte(`{"n":1,"prompt":"harbor","model":"grok-imagine"}`)
	hashA, err := CanonicalGrokMediaImageRequestHash(GrokMediaEndpointImagesGenerations, "application/json; charset=utf-8", jsonA)
	require.NoError(t, err)
	hashB, err := CanonicalGrokMediaImageRequestHash(GrokMediaEndpointImagesGenerations, "application/json", jsonB)
	require.NoError(t, err)
	require.Equal(t, hashA, hashB)

	bodyA, contentTypeA := buildImageEditMultipart(t, "boundary-a", false)
	bodyB, contentTypeB := buildImageEditMultipart(t, "boundary-b", true)
	hashA, err = CanonicalGrokMediaImageRequestHash(GrokMediaEndpointImagesEdits, contentTypeA, bodyA)
	require.NoError(t, err)
	hashB, err = CanonicalGrokMediaImageRequestHash(GrokMediaEndpointImagesEdits, contentTypeB, bodyB)
	require.NoError(t, err)
	require.Equal(t, hashA, hashB, "multipart boundary and part order must not change the canonical request")
}

func TestGrokMediaImageMultipartTemporaryFilenamesAreNonSemanticButImageOrderIsSemantic(t *testing.T) {
	for _, endpoint := range []GrokMediaEndpoint{GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits} {
		t.Run(string(endpoint), func(t *testing.T) {
			firstBody, firstContentType := buildGrokImageMultipart(t, "boundary-temp-a", "wj-url-ref-928374.tmp", "wj-url-ref-111111.tmp", [][]byte{[]byte("URL-REF-A"), []byte("URL-REF-B")}, false, "paint sails")
			retryBody, retryContentType := buildGrokImageMultipart(t, "boundary-temp-b", "wj-url-ref-abcdef.tmp", "wj-url-ref-fedcba.tmp", [][]byte{[]byte("URL-REF-A"), []byte("URL-REF-B")}, true, "paint sails")
			firstHash, err := CanonicalGrokMediaImageRequestHash(endpoint, firstContentType, firstBody)
			require.NoError(t, err)
			retryHash, err := CanonicalGrokMediaImageRequestHash(endpoint, retryContentType, retryBody)
			require.NoError(t, err)
			require.Equal(t, firstHash, retryHash, "temporary filenames, multipart boundaries, and cross-field ordering must not affect replay")

			swappedBody, swappedContentType := buildGrokImageMultipart(t, "boundary-order", "other-a.tmp", "other-b.tmp", [][]byte{[]byte("URL-REF-B"), []byte("URL-REF-A")}, false, "paint sails")
			swappedHash, err := CanonicalGrokMediaImageRequestHash(endpoint, swappedContentType, swappedBody)
			require.NoError(t, err)
			require.NotEqual(t, firstHash, swappedHash, "same-field image ordering is semantic")

			changedBytesBody, changedBytesContentType := buildGrokImageMultipart(t, "boundary-bytes", "other-a.tmp", "other-b.tmp", [][]byte{[]byte("URL-REF-A"), []byte("URL-REF-CHANGED")}, false, "paint sails")
			changedBytesHash, err := CanonicalGrokMediaImageRequestHash(endpoint, changedBytesContentType, changedBytesBody)
			require.NoError(t, err)
			require.NotEqual(t, firstHash, changedBytesHash, "uploaded bytes are semantic")

			changedPromptBody, changedPromptContentType := buildGrokImageMultipart(t, "boundary-prompt", "other-a.tmp", "other-b.tmp", [][]byte{[]byte("URL-REF-A"), []byte("URL-REF-B")}, false, "remove sails")
			changedPromptHash, err := CanonicalGrokMediaImageRequestHash(endpoint, changedPromptContentType, changedPromptBody)
			require.NoError(t, err)
			require.NotEqual(t, firstHash, changedPromptHash, "text fields are semantic")

			repo := &grokImageCreateRepoStub{}
			svc := &OpenAIGatewayService{grokMediaImageCreateRepo: repo}
			groupID := int64(17)
			first, err := svc.ClaimGrokMediaImageCreate(context.Background(), &groupID, endpoint, "wenjing-random-temp-name", firstContentType, firstBody, 31, 41)
			require.NoError(t, err)
			replayed, err := svc.ClaimGrokMediaImageCreate(context.Background(), &groupID, endpoint, "wenjing-random-temp-name", retryContentType, retryBody, 31, 41)
			require.NoError(t, err)
			require.Equal(t, first.RequestHash, replayed.RequestHash)
			_, err = svc.ClaimGrokMediaImageCreate(context.Background(), &groupID, endpoint, "wenjing-random-temp-name", changedBytesContentType, changedBytesBody, 31, 41)
			require.ErrorIs(t, err, ErrGrokMediaImageIdempotencyConflict)
		})
	}
}

func buildGrokImageMultipart(t *testing.T, boundary, firstFilename, secondFilename string, images [][]byte, reverseCrossFields bool, prompt string) ([]byte, string) {
	t.Helper()
	require.Len(t, images, 2)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	require.NoError(t, writer.SetBoundary(boundary))
	writeFields := func() {
		require.NoError(t, writer.WriteField("model", "grok-imagine"))
		require.NoError(t, writer.WriteField("prompt", prompt))
	}
	writeImages := func() {
		for index, filename := range []string{firstFilename, secondFilename} {
			part, err := writer.CreateFormFile("image", filename)
			require.NoError(t, err)
			_, err = part.Write(images[index])
			require.NoError(t, err)
		}
	}
	if reverseCrossFields {
		writeImages()
		writeFields()
	} else {
		writeFields()
		writeImages()
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes(), writer.FormDataContentType()
}

func buildImageEditMultipart(t *testing.T, boundary string, reverse bool) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	require.NoError(t, writer.SetBoundary(boundary))
	writePrompt := func() { require.NoError(t, writer.WriteField("prompt", "add sails")) }
	writeImage := func() {
		part, err := writer.CreateFormFile("image", "ship.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("PNG-CONTENT"))
		require.NoError(t, err)
	}
	if reverse {
		writeImage()
		writePrompt()
	} else {
		writePrompt()
		writeImage()
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes(), writer.FormDataContentType()
}

func TestGrokMediaImageCreateCrashReplayAndCallerIsolation(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	repo := &grokImageCreateRepoStub{}
	videoOwnerRepo := &grokVideoOwnerRepoStub{}
	upstream := &grokImageIdempotentUpstream{byKey: make(map[string]string)}
	groupID := int64(7)
	account := &Account{ID: 63, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "test-key", "base_url": "https://xai.test/v1"}}
	body := []byte(`{"model":"grok-imagine","prompt":"harbor"}`)
	svc := &OpenAIGatewayService{grokMediaImageCreateRepo: repo, grokMediaVideoOwnerRepo: videoOwnerRepo, httpUpstream: upstream}
	first, err := svc.ClaimGrokMediaImageCreate(context.Background(), &groupID, GrokMediaEndpointImagesGenerations, "wj-image-42", "application/json", body, 41, 51)
	require.NoError(t, err)
	boundID, err := svc.BindGrokMediaImageCreateAccount(context.Background(), first, account.ID)
	require.NoError(t, err)
	require.Equal(t, account.ID, boundID)

	firstRecorder := httptest.NewRecorder()
	firstContext, _ := gin.CreateTestContext(firstRecorder)
	firstContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	SetGrokMediaUpstreamIdempotencyKey(firstContext, first.UpstreamIdempotencyKey)
	_, err = svc.ForwardGrokMedia(context.Background(), firstContext, account, GrokMediaEndpointImagesGenerations, "", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, 1, upstream.uniqueCreates)
	require.False(t, firstContext.Writer.Written(), "response must stay buffered until durable completion")

	// Simulate process loss after upstream acceptance but before response
	// persistence. A fresh process recovers the same account and upstream key.
	restarted := &OpenAIGatewayService{grokMediaImageCreateRepo: repo, grokMediaVideoOwnerRepo: videoOwnerRepo, httpUpstream: upstream}
	reordered := []byte(`{"prompt":"harbor","model":"grok-imagine"}`)
	recovered, err := restarted.ClaimGrokMediaImageCreate(context.Background(), &groupID, GrokMediaEndpointImagesGenerations, "wj-image-42", "application/json", reordered, 41, 51)
	require.NoError(t, err)
	require.Equal(t, account.ID, recovered.AccountID)
	require.Equal(t, first.UpstreamIdempotencyKey, recovered.UpstreamIdempotencyKey)

	secondRecorder := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(secondRecorder)
	secondContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(reordered))
	SetGrokMediaUpstreamIdempotencyKey(secondContext, recovered.UpstreamIdempotencyKey)
	_, err = restarted.ForwardGrokMedia(context.Background(), secondContext, account, GrokMediaEndpointImagesGenerations, "", reordered, "application/json")
	require.NoError(t, err)
	require.Equal(t, 2, upstream.requests)
	require.Equal(t, 1, upstream.uniqueCreates, "stable upstream key must keep paid image creation exactly once")
	status, responseContentType, responseBody, err := BufferedGrokMediaResponse(secondContext)
	require.NoError(t, err)
	require.NoError(t, restarted.CompleteGrokMediaImageCreate(context.Background(), recovered, account.ID, status, responseContentType, responseBody))

	completed, err := restarted.ClaimGrokMediaImageCreate(context.Background(), &groupID, GrokMediaEndpointImagesGenerations, "wj-image-42", "application/json", body, 41, 51)
	require.NoError(t, err)
	require.True(t, completed.Completed())
	replayRecorder := httptest.NewRecorder()
	replayContext, _ := gin.CreateTestContext(replayRecorder)
	require.NoError(t, WriteGrokMediaImageCreateReplay(replayContext, completed))
	require.Equal(t, http.StatusOK, replayRecorder.Code)
	require.Equal(t, "true", replayRecorder.Header().Get("Idempotent-Replayed"))

	_, err = restarted.ClaimGrokMediaImageCreate(context.Background(), &groupID, GrokMediaEndpointImagesGenerations, "wj-image-42", "application/json", []byte(`{"model":"grok-imagine","prompt":"different"}`), 41, 51)
	require.ErrorIs(t, err, ErrGrokMediaImageIdempotencyConflict)
	otherCaller, err := restarted.ClaimGrokMediaImageCreate(context.Background(), &groupID, GrokMediaEndpointImagesGenerations, "wj-image-42", "application/json", body, 42, 51)
	require.NoError(t, err)
	require.NotEqual(t, completed.UpstreamIdempotencyKey, otherCaller.UpstreamIdempotencyKey)

	stored := repo.creates[grokImageCreateRepoKey(*completed)]
	require.WithinDuration(t, time.Now().Add(grokMediaImageCreateIdempotencyTTL), stored.ExpiresAt, 2*time.Second)
	require.Empty(t, videoOwnerRepo.owners, "image generation must never persist a video owner")
}

func TestGrokMediaImageCreateConcurrentClaimsShareAccountAndUpstreamKey(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	repo := &grokImageCreateRepoStub{}
	videoOwnerRepo := &grokVideoOwnerRepoStub{}
	upstream := &grokImageIdempotentUpstream{byKey: make(map[string]string)}
	svc := &OpenAIGatewayService{grokMediaImageCreateRepo: repo, grokMediaVideoOwnerRepo: videoOwnerRepo, httpUpstream: upstream}
	groupID := int64(9)
	body := []byte(`{"model":"grok-imagine","prompt":"fleet"}`)
	account := &Account{ID: 77, Platform: PlatformGrok, Type: AccountTypeAPIKey, Concurrency: 20, Credentials: map[string]any{"api_key": "test-key", "base_url": "https://xai.test/v1"}}
	const callers = 16
	records := make(chan *GrokMediaImageCreateRecord, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := svc.ClaimGrokMediaImageCreate(context.Background(), &groupID, GrokMediaEndpointImagesGenerations, "concurrent-image", "application/json", body, 1, 2)
			if err == nil {
				_, err = svc.BindGrokMediaImageCreateAccount(context.Background(), record, 77)
			}
			if err == nil {
				recorder := httptest.NewRecorder()
				requestContext, _ := gin.CreateTestContext(recorder)
				requestContext.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
				SetGrokMediaUpstreamIdempotencyKey(requestContext, record.UpstreamIdempotencyKey)
				_, err = svc.ForwardGrokMedia(context.Background(), requestContext, account, GrokMediaEndpointImagesGenerations, "", body, "application/json")
			}
			records <- record
			errs <- err
		}()
	}
	wg.Wait()
	close(records)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var upstreamKey string
	for record := range records {
		require.NotNil(t, record)
		require.Equal(t, int64(77), record.AccountID)
		if upstreamKey == "" {
			upstreamKey = record.UpstreamIdempotencyKey
		}
		require.Equal(t, upstreamKey, record.UpstreamIdempotencyKey)
	}
	require.Equal(t, 1, upstream.uniqueCreates, "concurrent retries must create one upstream image")
	require.Empty(t, videoOwnerRepo.owners, "concurrent image creates must not write video owners")
}
