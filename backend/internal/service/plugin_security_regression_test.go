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
REDACTED

func (r *pluginTokenRepository) List(context.Context) ([]*PluginInstallation, error) {
	if r.listErr != nil {
		return nil, r.listErr
REDACTED
	if r.installation == nil {
		return nil, nil
REDACTED
	copy := *r.installation
	return []*PluginInstallation{&copyREDACTED, nil
REDACTED

func (r *pluginTokenRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	if r.installation == nil {
		return nil, errors.New("插件不存在")
REDACTED
	copy := *r.installation
	return &copy, nil
REDACTED

type pluginTokenEncryptor struct{REDACTED

func (pluginTokenEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
REDACTED

func (pluginTokenEncryptor) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, "ENC:") {
		return "", errors.New("密文无效")
REDACTED
	return strings.TrimPrefix(ciphertext, "ENC:"), nil
REDACTED

func TestPluginUIAssetTokenCanBeResolvedByAnotherInstance(t *testing.T) {
	repo := &pluginTokenRepository{installation: &PluginInstallation{ID: 42REDACTEDREDACTED
	first := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{REDACTEDREDACTED
	second := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{REDACTEDREDACTED

	token, expires, err := first.CreateUIAssetToken(context.Background(), 42, 30*time.Minute)
REDACTED
	require.WithinDuration(t, time.Now().Add(30*time.Minute), expires, time.Second)

	pluginID, err := second.ResolveUIAssetToken(token)
REDACTED
	require.Equal(t, int64(42), pluginID)

	decoded, err := base64.RawURLEncoding.DecodeString(token)
REDACTED
	decoded[len(decoded)-1] ^= 1
	_, err = second.ResolveUIAssetToken(base64.RawURLEncoding.EncodeToString(decoded))
REDACTED
REDACTED

func TestPluginUIAssetTokenRejectsOtherEncryptedPayloads(t *testing.T) {
	repo := &pluginTokenRepository{installation: &PluginInstallation{ID: 42REDACTEDREDACTED
	manager := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{REDACTEDREDACTED

	encrypted, err := manager.encryptor.Encrypt(`{"version":1,"plugin_id":42,"expires":4102444800REDACTED`)
REDACTED
	token := base64.RawURLEncoding.EncodeToString([]byte(encrypted))

	_, err = manager.ResolveUIAssetToken(token)
	require.ErrorContains(t, err, "会话无效")
REDACTED

func TestPluginReconcileFailsClosedWhenDesiredStateCannotBeRead(t *testing.T) {
	manager := &PluginManager{
		repo:               &pluginTokenRepository{listErr: errors.New("数据库不可用")REDACTED,
		runtimes:           make(map[int64]*pluginRuntime),
		localInstallations: make(map[int64]*PluginInstallation),
REDACTED

	err := manager.reconcileOnce(context.Background())
	require.ErrorContains(t, err, "读取插件启用状态")
	require.True(t, manager.ShouldRouteOpenAIOAuth(&Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
REDACTED))

	request, requestErr := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", nil)
	require.NoError(t, requestErr)
	_, handled, routeErr := manager.RoundTripOpenAIOAuth(context.Background(), request, "", &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
REDACTED)
	require.True(t, handled)
	require.ErrorContains(t, routeErr, "插件不可用")
REDACTED

type normalizingPluginClient struct {
	pluginv1.TransportPluginClient
	normalized []byte
	applied    []byte
REDACTED

type pluginConfigRepository struct {
	PluginRepository
	installation *PluginInstallation
	encrypted    string
REDACTED

func (r *pluginConfigRepository) GetByID(context.Context, int64) (*PluginInstallation, error) {
	copy := *r.installation
	return &copy, nil
REDACTED

func (r *pluginConfigRepository) UpdateConfig(_ context.Context, _ int64, encrypted, expectedBinarySHA256 string) error {
	if expectedBinarySHA256 != r.installation.BinarySHA256 {
		return ErrPluginStateChanged
REDACTED
	r.encrypted = encrypted
	return nil
REDACTED

func (c *normalizingPluginClient) ValidateConfig(context.Context, *pluginv1.ValidateConfigRequest, ...grpc.CallOption) (*pluginv1.ValidateConfigResponse, error) {
	return &pluginv1.ValidateConfigResponse{Valid: true, NormalizedConfigJson: c.normalizedREDACTED, nil
REDACTED

func (c *normalizingPluginClient) ApplyConfig(_ context.Context, request *pluginv1.ApplyConfigRequest, _ ...grpc.CallOption) (*pluginv1.ApplyConfigResponse, error) {
	c.applied = append([]byte(nil), request.ConfigJson...)
	return &pluginv1.ApplyConfigResponse{Applied: trueREDACTED, nil
REDACTED

func TestPluginRuntimeReturnsAndAppliesNormalizedConfig(t *testing.T) {
	client := &normalizingPluginClient{normalized: []byte(`{"z":2,"a":1REDACTED`)REDACTED
	runtime := &pluginRuntime{api: clientREDACTED

	normalized, err := runtime.validateAndApplyNormalizedConfig(context.Background(), []byte(`{"input":trueREDACTED`))
REDACTED
	require.JSONEq(t, `{"a":1,"z":2REDACTED`, string(normalized))
	require.Equal(t, normalized, client.applied)
REDACTED

func TestPluginRuntimeRejectsInvalidNormalizedConfig(t *testing.T) {
	client := &normalizingPluginClient{normalized: []byte(`{"broken"`)REDACTED
	runtime := &pluginRuntime{api: clientREDACTED

	_, err := runtime.validateAndApplyNormalizedConfig(context.Background(), []byte(`{REDACTED`))
	require.ErrorContains(t, err, "规范化配置")
	require.Empty(t, client.applied)
REDACTED

func TestPluginManagerPersistsPluginNormalizedConfig(t *testing.T) {
	installation := &PluginInstallation{ID: 9, BinarySHA256: strings.Repeat("a", 64)REDACTED
	repo := &pluginConfigRepository{installation: installationREDACTED
	client := &normalizingPluginClient{normalized: []byte(`{"timeout":30,"enabled":trueREDACTED`)REDACTED
	manager := &PluginManager{
		repo: repo, encryptor: pluginTokenEncryptor{REDACTED,
		runtimes: map[int64]*pluginRuntime{9: {installation: installation, api: clientREDACTEDREDACTED,
REDACTED

	saved, err := manager.SaveConfig(context.Background(), 9, []byte(`{"enabled":falseREDACTED`))
REDACTED
	require.JSONEq(t, `{"enabled":true,"timeout":30REDACTED`, string(saved))
	plaintext, err := (pluginTokenEncryptor{REDACTED).Decrypt(repo.encrypted)
REDACTED
	require.JSONEq(t, string(saved), plaintext)
REDACTED

func TestPluginRequestSentErrorDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 7, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	transportErr := &PluginTransportError{Code: "UPSTREAM_EOF", Message: "eof", RequestSent: trueREDACTED

	result := (&OpenAIGatewayService{REDACTED).handleOpenAIUpstreamTransportError(context.Background(), c, account, transportErr, true)

	require.Same(t, transportErr, result)
	var failover *UpstreamFailoverError
	require.False(t, errors.As(result, &failover))
REDACTED

func TestPluginRPCAmbiguityPreventsReplayAfterMetadataDelivery(t *testing.T) {
	err := normalizePluginRPCError(context.Background(), "接收插件响应头", errors.New("连接已断开"), true)
	var transportErr *PluginTransportError
	require.ErrorAs(t, err, &transportErr)
	require.True(t, transportErr.RequestSent)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	result := (&OpenAIGatewayService{REDACTED).handleOpenAIUpstreamTransportError(context.Background(), c, account, err, true)
	require.Same(t, err, result)
REDACTED

func TestPluginRPCFailureBeforeStreamCreationAllowsFailover(t *testing.T) {
	err := normalizePluginRPCError(context.Background(), "创建插件转发流", errors.New("连接失败"), false)
	var transportErr *PluginTransportError
	require.ErrorAs(t, err, &transportErr)
	require.False(t, transportErr.RequestSent)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	account := &Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	result := (&OpenAIGatewayService{REDACTED).handleOpenAIUpstreamTransportError(context.Background(), c, account, err, true)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, result, &failover)
REDACTED

func TestNormalizePluginRPCErrorPreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := normalizePluginRPCError(ctx, "接收响应", errors.New("rpc error: code = Canceled"), true)
	require.ErrorIs(t, err, context.Canceled)
REDACTED

func TestPluginStartingStateUsesBoundedCrashRecoveryWindow(t *testing.T) {
	manager := &PluginManager{cfg: &config.Config{Plugins: config.PluginConfig{StartTimeoutSeconds: 15REDACTEDREDACTEDREDACTED

	require.False(t, manager.startingStateExpired(&PluginInstallation{UpdatedAt: time.Now().Add(-30 * time.Second)REDACTED))
	require.True(t, manager.startingStateExpired(&PluginInstallation{UpdatedAt: time.Now().Add(-2 * time.Minute)REDACTED))
REDACTED
