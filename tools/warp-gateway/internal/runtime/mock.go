package runtime

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
	socks5 "github.com/armon/go-socks5"
)

// MockManager starts in-process SOCKS5 listeners (no real WARP).
// Used for local integration tests of control plane + dial path.
type MockManager struct{}

func NewMockManager() *MockManager { return &MockManager{} }

func (m *MockManager) Name() string { return "mock" }

type mockHandle struct {
	ln   net.Listener
	addr string
	once sync.Once
}

func (h *mockHandle) LocalAddr() string { return h.addr }

func (h *mockHandle) Stop(ctx context.Context) error {
	var err error
	h.once.Do(func() {
		err = h.ln.Close()
	})
	return err
}

func (m *MockManager) Start(ctx context.Context, inst *store.Instance) (Handle, error) {
	addr := fmt.Sprintf("%s:%d", inst.ListenHost, inst.ListenPort)
	lc := net.ListenConfig{Control: reusePortControl}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		// Retry once after brief delay (port release race on restart/rotate).
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		ln, err = lc.Listen(context.Background(), "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen %s: %w", addr, err)
		}
	}

	conf := &socks5.Config{
		// Default Dial connects directly — fine for mock local tests.
		Logger: log.New(io.Discard, "", 0),
	}
	if inst.SocksAuthUser != "" {
		creds := socks5.StaticCredentials{
			inst.SocksAuthUser: inst.SocksAuthPass,
		}
		conf.Credentials = creds
		conf.AuthMethods = []socks5.Authenticator{socks5.UserPassAuthenticator{Credentials: creds}}
	}
	server, err := socks5.New(conf)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}

	h := &mockHandle{ln: ln, addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = server.ServeConn(c)
			}(conn)
		}
	}()

	// Ensure context cancellation stops listener when caller wants.
	go func() {
		<-ctx.Done()
		_ = h.Stop(context.Background())
	}()

	return h, nil
}

