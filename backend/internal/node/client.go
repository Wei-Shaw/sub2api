package node

// xiugai 添加节点功能

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// controlClient 封装对控制服务器的 HTTP 请求，支持 mTLS 双向证书认证。
type controlClient struct {
	// http 底层 HTTP 客户端，已配置 TLS 和超时。
	http *http.Client
	// baseURL 控制服务器 API 前缀，例如 https://127.0.0.1:8443/api/v1。
	baseURL string
}

// newControlClient 根据节点配置构建 controlClient。
// 当 Config.TLSCert/TLSKey 不为空时加载客户端证书；当 Config.TLSCA 不为空时
// 使用指定 CA 校验服务端证书，否则使用系统根证书池。
//
// 参数：
//   - cfg：节点配置，包含控制服务器地址和 TLS 证书路径。
//
// 返回值：
//   - *controlClient：构建成功的客户端。
//   - error：证书加载或解析失败时返回错误。
func newControlClient(cfg *Config) (*controlClient, error) {
	tlsCfg := &tls.Config{}

	// 如果同时配置了客户端证书和私钥，则加载以支持 mTLS。
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("load client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	// 如果指定了 CA 证书，则构建自定义根证书池以校验服务端。
	if cfg.TLSCA != "" {
		pem, err := os.ReadFile(cfg.TLSCA)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse CA cert: no valid PEM block found in %s", cfg.TLSCA)
		}
		tlsCfg.RootCAs = pool
	}

	return &controlClient{
		http: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   10 * time.Second,
		},
		baseURL: cfg.ControlAddr + "/api/v1",
	}, nil
}

// post 向控制服务器发送 POST 请求，请求体为 body 的 JSON 序列化结果。
// 仅接受 HTTP 200 作为成功响应，其他状态码均返回错误。
//
// 参数：
//   - ctx：请求上下文，用于超时和取消控制。
//   - path：相对于 baseURL 的接口路径，例如 "/heartbeat"。
//   - body：将被序列化为 JSON 的请求体对象。
//
// 返回值：
//   - error：网络错误、JSON 序列化失败或非 200 响应时返回错误。
func (c *controlClient) post(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	// 丢弃响应体以复用底层 TCP 连接。
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// end
