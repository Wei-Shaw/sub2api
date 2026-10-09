package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// 使用真实传输构建器和 HTTP/2 帧验证保活行为；虚拟时间保持生产超时值且无需实际等待。
func TestBuildUpstreamTransport_HTTP2PingPolicy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         string
		ackDelay     time.Duration
		finishDelay  time.Duration
		headersEarly bool
		wantLost     bool
		wantElapsed  time.Duration
	}{
		{"OpenAI延迟应答", upstreamProtocolModeOpenAIH2, 6 * time.Second, 22 * time.Second, false, false, 22 * time.Second},
		{"OpenAI已开始输出后延迟应答", upstreamProtocolModeOpenAIH2, 6 * time.Second, 22 * time.Second, true, false, 22 * time.Second},
		{"OpenAI正常应答但长时间无业务输出", upstreamProtocolModeOpenAIH2, 0, 40 * time.Second, false, false, 40 * time.Second},
		{"OpenAI无应答且未收到响应头", upstreamProtocolModeOpenAIH2, -1, time.Minute, false, true, 30 * time.Second},
		{"OpenAI无应答且已开始输出", upstreamProtocolModeOpenAIH2, -1, time.Minute, true, true, 30 * time.Second},
		{"长流保持原有超时", upstreamProtocolModeLongStreamH2, 6 * time.Second, 22 * time.Second, false, true, 15 * time.Second},
		{"长流正常应答", upstreamProtocolModeLongStreamH2, 0, 40 * time.Second, false, false, 40 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clientConn, serverConn := net.Pipe()
				defer func() { require.NoError(t, clientConn.Close()) }()
				defer func() { require.NoError(t, serverConn.Close()) }()
				peerDone := make(chan error, 1)
				go func() {
					err := serveHTTP2PingTestPeer(serverConn, tc.ackDelay, tc.finishDelay, tc.headersEarly)
					// 业务结果读完后客户端会关闭连接，此时尚在发送的 PING 应答可正常退出。
					if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
						err = nil
					}
					peerDone <- err
				}()

				tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, tc.mode)
				require.NoError(t, err)
				defer tr.CloseIdleConnections()
				// 仅把 TLS 建连替换为内存 HTTP/2 连接，保活参数仍来自生产构建器。
				tr.Protocols.SetHTTP1(false)
				tr.Protocols.SetUnencryptedHTTP2(true)
				tr.DialContext = func(context.Context, string, string) (net.Conn, error) {
					return clientConn, nil
				}
				client := &http.Client{Transport: tr, Timeout: 50 * time.Second}
				started := time.Now()
				resp, requestErr := client.Post("http://upstream.test/v1/responses", "application/json", strings.NewReader(`{"stream":true}`))
				resultErr := requestErr
				var body []byte
				if resp != nil {
					require.Equal(t, http.StatusOK, resp.StatusCode)
					require.Equal(t, 2, resp.ProtoMajor)
					body, resultErr = io.ReadAll(resp.Body)
					require.NoError(t, resp.Body.Close())
				}
				if tc.wantLost {
					require.ErrorContains(t, resultErr, "http2: client connection lost")
					if tc.headersEarly {
						require.NoError(t, requestErr)
						require.Equal(t, "data: first\n\n", string(body))
					} else {
						require.Nil(t, resp)
					}
				} else {
					require.NoError(t, resultErr)
					require.NotNil(t, resp)
					wantBody := "data: [DONE]\n\n"
					if tc.headersEarly {
						wantBody = "data: first\n\n" + wantBody
					}
					require.Equal(t, wantBody, string(body))
				}
				require.Equal(t, tc.wantElapsed, time.Since(started))
				require.NoError(t, clientConn.Close())
				require.NoError(t, <-peerDone)
			})
		})
	}
}

// 对端持续读取请求，只对 PING 的应答施加延迟；业务响应独立于 PING 定时发送。
func serveHTTP2PingTestPeer(conn net.Conn, ackDelay, finishDelay time.Duration, headersEarly bool) error {
	framer := http2.NewFramer(conn, conn)
	type readResult struct {
		frame http2.Frame
		err   error
	}
	// 缓冲握手帧，避免 net.Pipe 双向无缓冲写入造成 SETTINGS 握手互等。
	frames := make(chan readResult, 16)
	done := make(chan struct{})
	defer close(done)
	go func() {
		preface := make([]byte, len(http2.ClientPreface))
		_, err := io.ReadFull(conn, preface)
		if err == nil && string(preface) != http2.ClientPreface {
			err = errors.New("HTTP/2 客户端前言不匹配")
		}
		for {
			var frame http2.Frame
			if err == nil {
				frame, err = framer.ReadFrame()
			}
			select {
			case frames <- readResult{frame, err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	if err := framer.WriteSettings(); err != nil {
		return err
	}
	sendHeaders := func(streamID uint32) error {
		var block bytes.Buffer
		encoder := hpack.NewEncoder(&block)
		for _, field := range []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "content-type", Value: "text/event-stream"}} {
			if err := encoder.WriteField(field); err != nil {
				return err
			}
		}
		return framer.WriteHeaders(http2.HeadersFrameParam{StreamID: streamID, BlockFragment: block.Bytes(), EndHeaders: true})
	}
	var finishCh, ackCh <-chan time.Time
	var streamID uint32
	var pingData [8]byte
	for {
		select {
		case result := <-frames:
			if errors.Is(result.err, io.EOF) || errors.Is(result.err, net.ErrClosed) {
				return nil
			}
			if result.err != nil {
				return result.err
			}
			switch frame := result.frame.(type) {
			case *http2.SettingsFrame:
				if !frame.IsAck() {
					if err := framer.WriteSettingsAck(); err != nil {
						return err
					}
				}
			case *http2.HeadersFrame:
				if streamID != 0 {
					return fmt.Errorf("意外重放请求：流 %d", frame.StreamID)
				}
				streamID = frame.StreamID
				if headersEarly {
					if err := sendHeaders(streamID); err != nil {
						return err
					}
					if err := framer.WriteData(streamID, false, []byte("data: first\n\n")); err != nil {
						return err
					}
				}
				finishTimer := time.NewTimer(finishDelay)
				defer finishTimer.Stop()
				finishCh = finishTimer.C
			case *http2.PingFrame:
				if !frame.IsAck() && ackDelay >= 0 {
					pingData = frame.Data
					ackTimer := time.NewTimer(ackDelay)
					defer ackTimer.Stop()
					ackCh = ackTimer.C
				}
			}
		case <-ackCh:
			ackCh = nil
			if err := framer.WritePing(true, pingData); err != nil {
				return err
			}
		case <-finishCh:
			finishCh = nil
			if !headersEarly {
				if err := sendHeaders(streamID); err != nil {
					return err
				}
			}
			if err := framer.WriteData(streamID, true, []byte("data: [DONE]\n\n")); err != nil {
				return err
			}
		}
	}
}
