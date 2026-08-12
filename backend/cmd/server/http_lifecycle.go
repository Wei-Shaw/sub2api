package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
)

type httpServerLifecycle struct {
	server   *http.Server
	listener net.Listener
	stopped  bool
	mu       sync.Mutex
}

func newHTTPServerComponent(server *http.Server) runtime.Component {
	lifecycle := &httpServerLifecycle{server: server}
	return runtime.Component{
		Name:  "http-server",
		Roles: []runtime.Role{runtime.RoleAPI},
		Start: lifecycle.Start,
		Stop:  lifecycle.Stop,
	}
}

func (l *httpServerLifecycle) Start(context.Context) error {
	if l.server == nil {
		return errors.New("nil HTTP server")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return errors.New("HTTP server lifecycle cannot restart after shutdown")
	}
	if l.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", l.server.Addr)
	if err != nil {
		return err
	}
	l.listener = listener
	go func() {
		if err := l.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server stopped unexpectedly: %v", err)
		}
	}()
	return nil
}

func (l *httpServerLifecycle) Stop(ctx context.Context) error {
	if l.server == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener == nil {
		return nil
	}
	if err := l.server.Shutdown(ctx); err != nil {
		return err
	}
	l.listener = nil
	l.stopped = true
	return nil
}
