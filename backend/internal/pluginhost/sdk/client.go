package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client 是宿主能力 API（KV / Log / Config）的 HTTP 客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient 构造宿主能力 API 客户端：基址来自环境变量（非机密），
// token 来自宿主 stdin 握手（机密不走 env）。
func NewClient() (*Client, error) {
	baseURL := os.Getenv(EnvCapabilityURL)
	if baseURL == "" {
		return nil, fmt.Errorf("sdk: %s is not set (must be launched by sub2api plugin host)", EnvCapabilityURL)
	}
	hs, err := readHandshake()
	if err != nil {
		return nil, err
	}
	return NewClientWithToken(baseURL, hs.Token), nil
}

// NewClientWithToken 用显式基址与 token 构造能力客户端（宿主侧测试等
// 非常规拉起场景用；常规插件用 NewClient 从宿主握手取材）。
func NewClientWithToken(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Config 把宿主握手注入的插件私有配置解码到 out（未配置时为空对象）。
// 每次进程拉起都重新注入当时的最新配置；运行中读取最新值用 Client.Config。
func Config(out any) error {
	hs, err := readHandshake()
	if err != nil {
		return err
	}
	raw := hs.Config
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("sdk: decode plugin config: %w", err)
	}
	return nil
}

// KVGet 读取一个键；found=false 表示键不存在。
func (c *Client) KVGet(ctx context.Context, key string) (value string, found bool, err error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/kv/"+url.PathEscape(key), nil)
	if err != nil {
		return "", false, err
	}
	defer drainClose(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		var payload struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return "", false, fmt.Errorf("sdk: decode kv get response: %w", err)
		}
		return payload.Value, true, nil
	case http.StatusNotFound:
		return "", false, nil
	default:
		return "", false, unexpectedStatus("kv get", resp)
	}
}

// KVSet 写入或覆盖一个键。
func (c *Client) KVSet(ctx context.Context, key, value string) error {
	resp, err := c.do(ctx, http.MethodPut, "/v1/kv/"+url.PathEscape(key), strings.NewReader(value))
	if err != nil {
		return err
	}
	defer drainClose(resp)
	if resp.StatusCode != http.StatusNoContent {
		return unexpectedStatus("kv set", resp)
	}
	return nil
}

// KVDelete 删除一个键（不存在时也成功）。
func (c *Client) KVDelete(ctx context.Context, key string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/v1/kv/"+url.PathEscape(key), nil)
	if err != nil {
		return err
	}
	defer drainClose(resp)
	if resp.StatusCode != http.StatusNoContent {
		return unexpectedStatus("kv delete", resp)
	}
	return nil
}

// KVKeys 列出本插件命名空间内的键（prefix 为空时列全部）。
func (c *Client) KVKeys(ctx context.Context, prefix string) ([]string, error) {
	path := "/v1/kv"
	if prefix != "" {
		path += "?prefix=" + url.QueryEscape(prefix)
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer drainClose(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("kv keys", resp)
	}
	var payload struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("sdk: decode kv keys response: %w", err)
	}
	return payload.Keys, nil
}

// Log 把一条结构化日志写入宿主日志流（level 取 debug/info/warn/error）。
func (c *Client) Log(ctx context.Context, level, message string, fields map[string]any) error {
	body, err := json.Marshal(map[string]any{
		"level":   level,
		"message": message,
		"fields":  fields,
	})
	if err != nil {
		return fmt.Errorf("sdk: marshal log payload: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/v1/log", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer drainClose(resp)
	if resp.StatusCode != http.StatusNoContent {
		return unexpectedStatus("log", resp)
	}
	return nil
}

// Config 读取插件当前的私有配置并解码到 out（读的是宿主 DB 中的最新值；
// 与启动时经 stdin 握手注入的配置相比可能已被管理员更新）。
func (c *Client) Config(ctx context.Context, out any) error {
	resp, err := c.do(ctx, http.MethodGet, "/v1/config", nil)
	if err != nil {
		return err
	}
	defer drainClose(resp)
	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("config", resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("sdk: decode config response: %w", err)
	}
	return nil
}

// do 发送带 Bearer token 的能力请求。
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("sdk: build capability request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sdk: capability request %s %s: %w", method, path, err)
	}
	return resp, nil
}

// unexpectedStatus 把非预期状态码转成带响应片段的错误。
func unexpectedStatus(op string, resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("sdk: %s: unexpected status %d: %s", op, resp.StatusCode, string(snippet))
}

// drainClose 读尽并关闭响应体，保证连接可复用。
func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
