package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPServerComponentBindsAndReleasesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	server := &http.Server{Addr: addr, Handler: http.NewServeMux()}
	component := newHTTPServerComponent(server)
	require.NoError(t, component.Start(context.Background()))

	connected, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	require.NoError(t, connected.Close())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, component.Stop(ctx))

	listener, err = net.Listen("tcp", addr)
	require.NoError(t, err)
	require.NoError(t, listener.Close())
	require.Error(t, component.Start(context.Background()))
}
