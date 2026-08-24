package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
)

type pluginRuntime struct {
	installation *PluginInstallation
	client       *hcplugin.Client
	api          pluginv1.TransportPluginClient
	inFlight     atomic.Int64
	draining     atomic.Bool
	done         chan struct{REDACTED
	doneOnce     sync.Once
REDACTED

func startPluginRuntime(ctx context.Context, installation *PluginInstallation, startTimeout time.Duration, socketDir string) (*pluginRuntime, error) {
	if installation == nil {
		return nil, errors.New("插件安装记录为空")
REDACTED
	checksum, err := hex.DecodeString(installation.BinarySHA256)
	if err != nil || len(checksum) != sha256.Size {
		return nil, errors.New("插件二进制哈希无效")
REDACTED
	cmd := exec.CommandContext(context.WithoutCancel(ctx), installation.BinaryPath)
	client := hcplugin.NewClient(&hcplugin.ClientConfig{
		HandshakeConfig:  pluginv1.HandshakeConfig,
		Plugins:          pluginv1.ClientPluginMap(),
		Cmd:              cmd,
		AllowedProtocols: []hcplugin.Protocol{hcplugin.ProtocolGRPCREDACTED,
		StartTimeout:     startTimeout,
		SecureConfig: &hcplugin.SecureConfig{
			Checksum: checksum,
			Hash:     sha256.New(),
	REDACTED,
		Logger:           hclog.NewNullLogger(),
		SyncStdout:       io.Discard,
		SyncStderr:       io.Discard,
		UnixSocketConfig: &hcplugin.UnixSocketConfig{TempDir: socketDirREDACTED,
		SkipHostEnv:      true,
REDACTED)
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("启动插件进程: %w", err)
REDACTED
	dispensed, err := rpcClient.Dispense(pluginv1.TransportPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("获取插件传输能力: %w", err)
REDACTED
	api, ok := dispensed.(pluginv1.TransportPluginClient)
	if !ok {
		client.Kill()
		return nil, errors.New("插件未实现传输 gRPC 客户端")
REDACTED
	runtime := &pluginRuntime{
		installation: installation,
		client:       client,
		api:          api,
		done:         make(chan struct{REDACTED),
REDACTED
	infoCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	info, err := api.GetInfo(infoCtx, &pluginv1.GetInfoRequest{REDACTED)
	if err != nil {
		runtime.kill()
		return nil, fmt.Errorf("读取插件信息: %w", err)
REDACTED
	if info.PluginId != installation.PluginKey || info.PluginVersion != installation.Version ||
		info.ProtocolVersion != pluginv1.ProtocolVersion || info.TransportApiVersion != pluginv1.TransportAPIVersion {
		runtime.kill()
		return nil, errors.New("插件运行时信息与已校验清单不一致")
REDACTED
	health, err := api.Health(infoCtx, &pluginv1.HealthRequest{REDACTED)
	if err != nil || !health.Healthy {
		runtime.kill()
		if err != nil {
			return nil, fmt.Errorf("插件健康检查失败: %w", err)
	REDACTED
		return nil, fmt.Errorf("插件不健康: %s", health.Message)
REDACTED
	return runtime, nil
REDACTED

func (r *pluginRuntime) validateAndApplyConfig(ctx context.Context, configJSON []byte) error {
	_, err := r.validateAndApplyNormalizedConfig(ctx, configJSON)
	return err
REDACTED

func (r *pluginRuntime) validateAndApplyNormalizedConfig(ctx context.Context, configJSON []byte) ([]byte, error) {
	validation, err := r.api.ValidateConfig(ctx, &pluginv1.ValidateConfigRequest{ConfigJson: configJSONREDACTED)
	if err != nil {
		return nil, fmt.Errorf("插件配置校验失败: %w", err)
REDACTED
	if !validation.Valid {
		return nil, fmt.Errorf("插件配置无效: %s", validation.Message)
REDACTED
	if len(validation.NormalizedConfigJson) > 0 {
		configJSON = validation.NormalizedConfigJson
REDACTED
	if len(configJSON) == 0 || len(configJSON) > pluginConfigMaxBytes || !json.Valid(configJSON) {
		return nil, errors.New("插件返回的规范化配置不是有效且大小受限的 JSON")
REDACTED
	var normalized any
	if err := json.Unmarshal(configJSON, &normalized); err != nil {
		return nil, fmt.Errorf("解析插件规范化配置: %w", err)
REDACTED
	configJSON, err = json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("序列化插件规范化配置: %w", err)
REDACTED
	applied, err := r.api.ApplyConfig(ctx, &pluginv1.ApplyConfigRequest{ConfigJson: configJSONREDACTED)
	if err != nil {
		return nil, fmt.Errorf("应用插件配置失败: %w", err)
REDACTED
	if !applied.Applied {
		return nil, fmt.Errorf("插件拒绝应用配置: %s", applied.Message)
REDACTED
	return configJSON, nil
REDACTED

func (r *pluginRuntime) beginRequest() bool {
	if r == nil || r.draining.Load() {
		return false
REDACTED
	r.inFlight.Add(1)
	if r.draining.Load() {
		r.finishRequest()
		return false
REDACTED
	return true
REDACTED

func (r *pluginRuntime) finishRequest() {
	if r.inFlight.Add(-1) == 0 && r.draining.Load() {
		r.doneOnce.Do(func() { close(r.done) REDACTED)
REDACTED
REDACTED

func (r *pluginRuntime) drain(timeout time.Duration) {
	if r == nil {
		return
REDACTED
	r.draining.Store(true)
	if r.inFlight.Load() == 0 {
		r.doneOnce.Do(func() { close(r.done) REDACTED)
REDACTED
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-r.done:
	case <-timer.C:
REDACTED
	r.kill()
REDACTED

func (r *pluginRuntime) kill() {
	if r != nil && r.client != nil {
		r.client.Kill()
REDACTED
REDACTED

func (r *pluginRuntime) roundTrip(ctx context.Context, request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if request == nil || request.URL == nil || account == nil {
		return nil, errors.New("插件出站请求参数不完整")
REDACTED
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := r.api.Forward(streamCtx)
	if err != nil {
		cancel()
		return nil, normalizePluginRPCError(ctx, "创建插件转发流", err, false)
REDACTED
	requestID := strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatInt(account.ID, 36)
	if err := stream.Send(&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{
		RequestId:          requestID,
		Method:             request.Method,
		Url:                request.URL.String(),
		Host:               request.Host,
		Headers:            headersToPlugin(request.Header),
		ProxyUrl:           proxyURL,
		AccountId:          account.ID,
		AccountConcurrency: int32(account.Concurrency),
		Platform:           account.Platform,
		AccountType:        account.Type,
		ContentLength:      request.ContentLength,
		HasBody:            request.Body != nil && request.Body != http.NoBody,
REDACTEDREDACTEDREDACTED); err != nil {
		cancel()
		// gRPC Send 返回错误时无法证明服务端没有收到元数据，必须禁止自动重放。
		return nil, normalizePluginRPCError(ctx, "发送插件请求元数据", err, true)
REDACTED
	sendErr := make(chan error, 1)
	go func() {
		err := sendPluginRequestBody(stream, request.Body)
		sendErr <- err
		if err != nil {
			cancel()
	REDACTED
REDACTED()

	first, err := stream.Recv()
	if err != nil {
		cancel()
		select {
		case bodyErr := <-sendErr:
			if bodyErr != nil {
				return nil, normalizePluginRPCError(ctx, "发送插件请求体", bodyErr, true)
		REDACTED
		default:
	REDACTED
		return nil, normalizePluginRPCError(ctx, "接收插件响应头", err, true)
REDACTED
	if frameError := first.GetError(); frameError != nil {
		cancel()
		return nil, &PluginTransportError{Code: frameError.Code, Message: frameError.Message, RequestSent: frameError.RequestSentREDACTED
REDACTED
	start := first.GetStart()
	if start == nil || start.StatusCode < 100 || start.StatusCode > 599 {
		cancel()
		return nil, &PluginTransportError{
			Code:        "PLUGIN_INVALID_RESPONSE",
			Message:     "插件未返回有效的 HTTP 响应头",
			RequestSent: true,
	REDACTED
REDACTED
	pipeReader, pipeWriter := io.Pipe()
	body := &pluginResponseBody{
		reader: pipeReader,
		cancel: cancel,
		done:   r.finishRequest,
REDACTED
	go receivePluginResponseBody(stream, pipeWriter, sendErr)
	return &http.Response{
		Status:        start.Status,
		StatusCode:    int(start.StatusCode),
		Proto:         start.Protocol,
		ProtoMajor:    int(start.ProtocolMajor),
		ProtoMinor:    int(start.ProtocolMinor),
		Header:        headersFromPlugin(start.Headers),
		Body:          body,
		ContentLength: start.ContentLength,
		Request:       request,
REDACTED, nil
REDACTED

type PluginTransportError struct {
	Code        string
	Message     string
	RequestSent bool
REDACTED

func (e *PluginTransportError) Error() string {
	if e == nil {
		return "插件传输失败"
REDACTED
	code := strings.Map(func(value rune) rune {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' || value == '-' || value == '.' {
			return value
	REDACTED
		return -1
REDACTED, e.Code)
	if len(code) > 64 {
		code = code[:64]
REDACTED
	return fmt.Sprintf("插件传输失败 [%s]: %s", code, sanitizeUpstreamErrorMessage(e.Message))
REDACTED

func normalizePluginRPCError(ctx context.Context, operation string, err error, requestMayHaveBeenSent bool) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
REDACTED
	return &PluginTransportError{
		Code:        "PLUGIN_RPC_ERROR",
		Message:     fmt.Sprintf("%s: %v", operation, err),
		RequestSent: requestMayHaveBeenSent,
REDACTED
REDACTED

func sendPluginRequestBody(stream pluginv1.TransportPlugin_ForwardClient, body io.ReadCloser) error {
	if body != nil {
		defer func() { _ = body.Close() REDACTED()
		buffer := make([]byte, 32*1024)
		for {
			read, err := body.Read(buffer)
			if read > 0 {
				chunk := append([]byte(nil), buffer[:read]...)
				if sendErr := stream.Send(&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: chunkREDACTEDREDACTED); sendErr != nil {
					return sendErr
			REDACTED
		REDACTED
			if err == io.EOF {
				break
		REDACTED
			if err != nil {
				return err
		REDACTED
	REDACTED
REDACTED
	if err := stream.Send(&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyEnd{BodyEnd: trueREDACTEDREDACTED); err != nil {
		return err
REDACTED
	return stream.CloseSend()
REDACTED

func receivePluginResponseBody(stream pluginv1.TransportPlugin_ForwardClient, writer *io.PipeWriter, sendErr <-chan error) {
	defer func() { _ = writer.Close() REDACTED()
	for {
		frame, err := stream.Recv()
		if err == io.EOF {
			return
	REDACTED
		if err != nil {
			_ = writer.CloseWithError(normalizePluginRPCError(stream.Context(), "接收插件响应体", err, true))
			return
	REDACTED
		if chunk := frame.GetBodyChunk(); len(chunk) > 0 {
			if _, err := writer.Write(chunk); err != nil {
				return
		REDACTED
			continue
	REDACTED
		if frame.GetEnd() != nil {
			select {
			case err := <-sendErr:
				if err != nil {
					_ = writer.CloseWithError(normalizePluginRPCError(stream.Context(), "发送插件请求体", err, true))
			REDACTED
			default:
		REDACTED
			return
	REDACTED
		if frameError := frame.GetError(); frameError != nil {
			_ = writer.CloseWithError(&PluginTransportError{Code: frameError.Code, Message: frameError.Message, RequestSent: frameError.RequestSentREDACTED)
			return
	REDACTED
REDACTED
REDACTED

type pluginResponseBody struct {
	reader *io.PipeReader
	cancel context.CancelFunc
	done   func()
	once   sync.Once
REDACTED

func (b *pluginResponseBody) Read(data []byte) (int, error) {
	return b.reader.Read(data)
REDACTED

func (b *pluginResponseBody) Close() error {
	var err error
	b.once.Do(func() {
		b.cancel()
		err = b.reader.Close()
		b.done()
REDACTED)
	return err
REDACTED

func headersToPlugin(headers http.Header) map[string]*pluginv1.HeaderValues {
	out := make(map[string]*pluginv1.HeaderValues, len(headers))
	for key, values := range headers {
		out[key] = &pluginv1.HeaderValues{Values: append([]string(nil), values...)REDACTED
REDACTED
	return out
REDACTED

func headersFromPlugin(headers map[string]*pluginv1.HeaderValues) http.Header {
	out := make(http.Header, len(headers))
	for key, values := range headers {
		if values != nil {
			out[key] = append([]string(nil), values.Values...)
	REDACTED
REDACTED
	return out
REDACTED
