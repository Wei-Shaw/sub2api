package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// frontendBundleFetchTimeout 限制单次插件 entry.js / css 拉取的最大耗时.
	// 插件 bundle 通常 < 1MB, 单次 RTT 在毫秒级, 给 30s 足够覆盖偶发 GC 抖动.
	frontendBundleFetchTimeout = 30 * time.Second

	// frontendBundleMaxBytes 限制单个文件的最大字节数, 防御式上限避免内存爆炸.
	// 5MB 远大于一般 plugin entry bundle 的体积.
	frontendBundleMaxBytes = 5 * 1024 * 1024
)

// FrontendAsset 描述一次 GetFrontendBundle 的结果.
// ETag 由 sha256(data) 截断产生, 用于核心 HTTP 层的 If-None-Match 匹配.
type FrontendAsset struct {
	Data []byte
	ETag string
}

// ErrPluginNotRunning 在调用方请求一个未运行 / 不存在的插件时返回.
// 调用方可以据此返回 404, 而不是把它当成内部错误返回 500.
var ErrPluginNotRunning = errors.New("plugin not running")

// validatePluginAssetPath 拒绝可能用于路径穿越的请求.
// 允许子目录形如 "dist/entry.js", 但禁止 ".." 和绝对路径.
func validatePluginAssetPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("plugin asset path is empty")
	}
	clean := path.Clean("/" + p)
	if strings.HasPrefix(clean, "/..") {
		return "", fmt.Errorf("invalid plugin asset path: %q", p)
	}
	clean = strings.TrimPrefix(clean, "/")
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid plugin asset path: %q", p)
	}
	return clean, nil
}

// FetchFrontendAsset 通过插件子进程的 gRPC GetFrontendBundle 流读取一个文件.
//
// 调用方需要保证 pluginName / assetPath 来自可信的来源 (manifest 或受控的 HTTP
// 路径), validatePluginAssetPath 会再次防御式校验, 拒绝 ".." 等穿越尝试.
//
// 返回值:
//   - asset: 文件内容 + ETag, 调用方可直接落入 HTTP 响应
//   - error: ErrPluginNotRunning 表示插件不存在 / 未运行 (可视作 404), 其它错误为 500
func (m *PluginManager) FetchFrontendAsset(ctx context.Context, pluginName, assetPath string) (*FrontendAsset, error) {
	cleanPath, err := validatePluginAssetPath(assetPath)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	inst, ok := m.plugins[pluginName]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrPluginNotRunning
	}

	stub, err := snapshotLifecycle(inst)
	if err != nil {
		return nil, err
	}

	rpcCtx, cancel := context.WithTimeout(ctx, frontendBundleFetchTimeout)
	defer cancel()

	stream, err := stub.GetFrontendBundle(rpcCtx, &pluginsdk.FrontendBundleRequest{Path: cleanPath})
	if err != nil {
		return nil, fmt.Errorf("plugin %q get frontend bundle: %w", pluginName, err)
	}

	data, err := drainBundleStream(stream)
	if err != nil {
		return nil, fmt.Errorf("plugin %q drain bundle stream: %w", pluginName, err)
	}

	return &FrontendAsset{
		Data: data,
		ETag: computeETag(data),
	}, nil
}

// snapshotLifecycle 在 inst.mu 内拷贝出 LifecycleStub, 避免在 gRPC 调用期间长时间持锁.
// 插件未 running 时返回 ErrPluginNotRunning.
func snapshotLifecycle(inst *PluginInstance) (pluginsdk.PluginLifecycleClient, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.State != StateRunning || inst.LifecycleStub == nil {
		return nil, ErrPluginNotRunning
	}
	return inst.LifecycleStub, nil
}

// drainBundleStream 把 gRPC server stream 上的 FileChunk 拼成单个 byte slice.
// 受 frontendBundleMaxBytes 限制, 超过会返回错误而不是默默截断.
func drainBundleStream(stream pluginsdk.PluginLifecycle_GetFrontendBundleClient) ([]byte, error) {
	buf := make([]byte, 0, 64*1024)
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			continue
		}
		if len(buf)+len(chunk.GetData()) > frontendBundleMaxBytes {
			return nil, fmt.Errorf("plugin frontend asset exceeds %d bytes", frontendBundleMaxBytes)
		}
		buf = append(buf, chunk.GetData()...)
		if chunk.GetEof() {
			// SDK 在最后一帧把 eof 置位; 显式 break 以便插件不发送额外 EOF 时也能退出
			break
		}
	}
	return buf, nil
}

// computeETag 用 sha256 前 16 字节 hex 表示 (32 chars), 足够稳定且短.
func computeETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// shortContentHashLen 是 entry_js_url / entry_css_url 上 ?v= 参数的长度
// (hex 字符数). 8 hex char = 4 byte = 2^32 唯一空间, 对单插件单 bundle 的
// 版本去重足够; 不需要全 SHA-256 — query 只是给浏览器一个稳定的"新版本"
// 信号, 真正的内容校验由 ETag (16 byte) 完成.
const shortContentHashLen = 8

// computeShortHash 返回 sha256 前 N/2 字节的 hex 表示 (N hex chars),
// 用作 plugin entry 资源 URL 的 cache-busting query 值. data 为空时返
// 回空串, 让上游"无 hash 时不拼 ?v="的退化路径直接判空即可.
func computeShortHash(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:shortContentHashLen/2])
}
