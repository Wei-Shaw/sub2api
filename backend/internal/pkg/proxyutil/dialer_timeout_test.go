package proxyutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SOCKS5 隧道建立（TCP 连接 + SOCKS 协商）必须有兜底超时：请求 ctx 通常没有
// deadline（流式请求随客户端断开才取消），代理黑洞时隧道建立会无限挂起。
func TestWithEstablishTimeout_AppliesDeadline(t *testing.T) {
	var captured context.Context
	dial := withEstablishTimeout(func(ctx context.Context, network, addr string) (net.Conn, error) {
		captured = ctx
		return nil, errors.New("intercepted")
	})

	_, err := dial(context.Background(), "tcp", "example.com:443")
	require.Error(t, err)
	require.NotNil(t, captured)

	deadline, ok := captured.Deadline()
	require.True(t, ok, "无 deadline 的 ctx 必须补隧道建立超时")
	require.WithinDuration(t, time.Now().Add(tunnelEstablishTimeout), deadline, 2*time.Second)
}

// 调用方已有更短 deadline 时不得放宽。
func TestWithEstablishTimeout_KeepsShorterCallerDeadline(t *testing.T) {
	var captured context.Context
	dial := withEstablishTimeout(func(ctx context.Context, network, addr string) (net.Conn, error) {
		captured = ctx
		return nil, errors.New("intercepted")
	})

	callerDeadline := time.Now().Add(500 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()

	_, _ = dial(ctx, "tcp", "example.com:443")

	deadline, ok := captured.Deadline()
	require.True(t, ok)
	require.False(t, deadline.After(callerDeadline.Add(50*time.Millisecond)),
		"包络不得放宽调用方已有的更短 deadline")
}

// ConfigureTransportProxy 的 SOCKS5 分支必须应用隧道建立超时包络。
func TestConfigureTransportProxy_SOCKS5DialHasEstablishDeadline(t *testing.T) {
	// 一个接受连接但永不回应的本地"代理"，模拟黑洞。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 1024)
				for {
					if _, err := c.Read(buf); err != nil {
						_ = c.Close()
						return
					}
				}
			}(conn)
		}
	}()

	httpTransport := &http.Transport{}
	proxyURL, err := url.Parse("socks5://" + ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, ConfigureTransportProxy(httpTransport, proxyURL))
	require.NotNil(t, httpTransport.DialContext)

	// 带短 deadline 的 ctx 必须被尊重（挂起代理下快速失败，而非无限等待）。
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := httpTransport.DialContext(ctx, "tcp", "example.com:443")
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err, "挂起的 SOCKS5 代理必须返回错误")
	case <-time.After(3 * time.Second):
		t.Fatal("SOCKS5 隧道建立未受 ctx deadline 约束（挂起）")
	}
}
