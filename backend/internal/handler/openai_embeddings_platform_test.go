//go:build unit

package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type embeddingsRecordingUpstream struct {
	service.HTTPUpstream
	mu       sync.Mutex
	urls     []string
	accounts []int64
}

func (u *embeddingsRecordingUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.urls = append(u.urls, req.URL.String())
	u.accounts = append(u.accounts, accountID)
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"data":[{"embedding":[0.1]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`)),
	}, nil
}

func embeddingsAPIKeyAccount(id int64, platform, baseURL string) service.Account {
	return service.Account{
		ID: id, Name: "emb", Platform: platform, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": baseURL},
	}
}

// 国产平台分组的 embeddings 请求须调度分组平台的账号，而不是调度器默认的 openai 平台。
func TestEmbeddingsDomesticGroupSelectsGroupPlatformAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &embeddingsRecordingUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	gatewayService := service.NewOpenAIGatewayService(
		openAIImagesFailoverAccountRepo{accounts: []service.Account{
			embeddingsAPIKeyAccount(1, service.PlatformOpenAI, "http://openai.example"),
			embeddingsAPIKeyAccount(2, service.PlatformZhipu, "http://zhipu.example/api/paas/v4"),
		}},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	h := NewOpenAIGatewayHandler(
		gatewayService,
		service.NewConcurrencyService(nil),
		billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil,
		cfg,
	)

	groupID := int64(4100)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader([]byte(`{"model":"embedding-3","input":"hi"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      41,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformZhipu},
		User:    &service.User{ID: 42},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})

	h.Embeddings(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []int64{2}, upstream.accounts)
	require.Equal(t, []string{"http://zhipu.example/api/paas/v4/embeddings"}, upstream.urls)
}
