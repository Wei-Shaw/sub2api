package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type pluginTokenRepository struct {
	PluginRepository
	installation *PluginInstallation
	listErr      error
}

func (r *pluginTokenRepository) List(context.Context) ([]*PluginInstallation, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.installation == nil {
		return nil, nil
	}
	copy := *r.installation
	return []*PluginInstallation{&copy}, nil
}

func (r *pluginTokenRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	if r.installation == nil {
		return nil, errors.New("插件不存在")
	}
	copy := *r.installation
	return &copy, nil
}

type pluginTokenEncryptor struct{}

func (pluginTokenEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
}

func (pluginTokenEncryptor) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, "ENC:") {
		return "", errors.New("密文无效")
	}
	return strings.TrimPrefix(ciphertext, "ENC:"), nil
}

func TestPluginUIAssetTokenCanBeResolvedByAnotherInstance(t *testing.T) {
	repo := &pluginTokenRepository{installation: &PluginInstallation{ID: 42}}
	first := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{}}
	second := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{}}

	token, expires, err := first.CreateUIAssetToken(context.Background(), 42, 30*time.Minute)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(30*time.Minute), expires, time.Second)

	pluginID, err := second.ResolveUIAssetToken(token)
	require.NoError(t, err)
	require.Equal(t, int64(42), pluginID)

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	decoded[len(decoded)-1] ^= 1
	_, err = second.ResolveUIAssetToken(base64.RawURLEncoding.EncodeToString(decoded))
	require.Error(t, err)
}

func TestPluginUIAssetTokenRejectsOtherEncryptedPayloads(t *testing.T) {
	repo := &pluginTokenRepository{installation: &PluginInstallation{ID: 42}}
	manager := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{}}

	encrypted, err := manager.encryptor.Encrypt(`{"version":1,"plugin_id":42,"expires":4102444800}`)
	require.NoError(t, err)
	token := base64.RawURLEncoding.EncodeToString([]byte(encrypted))

	_, err = manager.ResolveUIAssetToken(token)
	require.ErrorContains(t, err, "会话无效")
}

func TestPluginReconcileFailsClosedWhenDesiredStateCannotBeRead(t *testing.T) {
	manager := &PluginManager{
		repo:               &pluginTokenRepository{listErr: errors.New("数据库不可用")},
		runtimes:           make(map[int64]*pluginRuntime),
		localInstallations: make(map[int64]*PluginInstallation),
	}

	err := manager.reconcileOnce(context.Background())
	require.ErrorContains(t, err, "读取插件启用状态")
	require.True(t, manager.ShouldRouteOpenAIOAuth(&Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
	}))

	request, requestErr := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
	require.NoError(t, requestErr)
	_, handled, routeErr := manager.RoundTripOpenAIOAuth(context.Background(), request, "", &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
	})
	require.True(t, handled)
	require.ErrorContains(t, routeErr, "插件不可用")
}

type normalizingPluginClient struct {
	pluginv1.TransportPluginClient
	normalized []byte
	applied    []byte
}

type pluginConfigRepository struct {
	PluginRepository
	installation *PluginInstallation
	encrypted    string
}

func (r *pluginConfigRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	copy := *r.installation
	return &copy, nil
}

func (r *pluginConfigRepository) UpdateConfig(_ context.Context, _ int64, encrypted, expectedBinarySHA256 string) error {
	if expectedBinarySHA256 != r.installation.BinarySHA256 {
		return ErrPluginStateChanged
	}
	r.encrypted = encrypted
	return nil
}

func (c *normalizingPluginClient) ValidateConfig(context.Context, *pluginv1.ValidateConfigRequest, ...grpc.CallOption) (*pluginv1.ValidateConfigResponse, error) {
	return &pluginv1.ValidateConfigResponse{Valid: true, NormalizedConfigJson: c.normalized}, nil
}

func (c *normalizingPluginClient) ApplyConfig(_ context.Context, request *pluginv1.ApplyConfigRequest, _ ...grpc.CallOption) (*pluginv1.ApplyConfigResponse, error) {
	c.applied = append([]byte(nil), request.ConfigJson...)
	return &pluginv1.ApplyConfigResponse{Applied: true}, nil
}

func TestPluginRuntimeReturnsAndAppliesNormalizedConfig(t *testing.T) {
	client := &normalizingPluginClient{normalized: []byte(`{"z":2,"a":1}`)}
	runtime := &pluginRuntime{api: client}

	normalized, err := runtime.validateAndApplyNormalizedConfig(context.Background(), []byte(`{"input":true}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"a":1,"z":2}`, string(normalized))
	require.Equal(t, normalized, client.applied)
}

func TestPluginRuntimeRejectsInvalidNormalizedConfig(t *testing.T) {
	client := &normalizingPluginClient{normalized: []byte(`{"broken"`)}
	runtime := &pluginRuntime{api: client}

	_, err := runtime.validateAndApplyNormalizedConfig(context.Background(), []byte(`{}`))
	require.ErrorContains(t, err, "规范化配置")
	require.Empty(t, client.applied)
}

func TestPluginManagerPersistsPluginNormalizedConfig(t *testing.T) {
	installation := &PluginInstallation{ID: 9, BinarySHA256: strings.Repeat("a", 64)}
	repo := &pluginConfigRepository{installation: installation}
	client := &normalizingPluginClient{normalized: []byte(`{"timeout":30,"enabled":true}`)}
	manager := &PluginManager{
		repo: repo, encryptor: pluginTokenEncryptor{},
		runtimes: map[int64]*pluginRuntime{9: {installation: installation, api: client}},
	}

	saved, err := manager.SaveConfig(context.Background(), 9, []byte(`{"enabled":false}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"enabled":true,"timeout":30}`, string(saved))
	plaintext, err := (pluginTokenEncryptor{}).Decrypt(repo.encrypted)
	require.NoError(t, err)
	require.JSONEq(t, string(saved), plaintext)
}

func TestPluginRequestSentErrorDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 7, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	transportErr := &PluginTransportError{Code: "UPSTREAM_EOF", Message: "eof", RequestSent: true}

	result := (&OpenAIGatewayService{}).handleOpenAIUpstreamTransportError(context.Background(), c, account, transportErr, true)

	require.Same(t, transportErr, result)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(result, &failover))
}

func TestPluginRPCAmbiguityPreventsReplayAfterMetadataDelivery(t *testing.T) {
	err := normalizePluginRPCError(context.Background(), "接收插件响应头", errors.New("连接已断开"), true)
	var transportErr *PluginTransportError
	require.ErrorAs(t, err, &transportErr)
	require.True(t, transportErr.RequestSent)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	result := (&OpenAIGatewayService{}).handleOpenAIUpstreamTransportError(context.Background(), c, account, err, true)
	require.Same(t, err, result)
}

func TestPluginRPCFailureBeforeStreamCreationAllowsFailover(t *testing.T) {
	err := normalizePluginRPCError(context.Background(), "创建插件转发流", errors.New("连接失败"), false)
	var transportErr *PluginTransportError
	require.ErrorAs(t, err, &transportErr)
	require.False(t, transportErr.RequestSent)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	result := (&OpenAIGatewayService{}).handleOpenAIUpstreamTransportError(context.Background(), c, account, err, true)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, result, &failover)
}

func TestNormalizePluginRPCErrorPreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := normalizePluginRPCError(ctx, "接收响应", errors.New("rpc error: code = Canceled"), true)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPluginStartingStateUsesBoundedCrashRecoveryWindow(t *testing.T) {
	manager := &PluginManager{cfg: &config.Config{Plugins: config.PluginConfig{StartTimeoutSeconds: 15}}}

	require.False(t, manager.startingStateExpired(&PluginInstallation{UpdatedAt: time.Now().Add(-30 * time.Second)}))
	require.True(t, manager.startingStateExpired(&PluginInstallation{UpdatedAt: time.Now().Add(-2 * time.Minute)}))
}
