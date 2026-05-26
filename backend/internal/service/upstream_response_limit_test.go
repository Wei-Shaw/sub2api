package service

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveUpstreamResponseReadLimit(t *testing.T) {
	t.Run("use default when config missing", func(t *testing.T) {
		require.Equal(t, defaultUpstreamResponseReadMaxBytes, resolveUpstreamResponseReadLimit(nil))
	})

	t.Run("use configured value", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.UpstreamResponseReadMaxBytes = 1234
		require.Equal(t, int64(1234), resolveUpstreamResponseReadLimit(cfg))
	})
}

func TestReadUpstreamResponseBodyLimited(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		body, err := readUpstreamResponseBodyLimited(bytes.NewReader([]byte("ok")), 2)
		require.NoError(t, err)
		require.Equal(t, []byte("ok"), body)
	})

	t.Run("exceeds limit", func(t *testing.T) {
		body, err := readUpstreamResponseBodyLimited(bytes.NewReader([]byte("toolong")), 3)
		require.Nil(t, body)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrUpstreamResponseBodyTooLarge))
	})
}

func TestReadUpstreamResponseBody(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		body, err := ReadUpstreamResponseBody(bytes.NewReader([]byte("ok")), nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []byte("ok"), body)
	})

	t.Run("exceeds limit calls onTooLarge", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.UpstreamResponseReadMaxBytes = 3

		called := false
		onTooLarge := func(_ *gin.Context) { called = true }

		body, err := ReadUpstreamResponseBody(bytes.NewReader([]byte("toolong")), cfg, nil, onTooLarge)
		require.Nil(t, body)
		require.True(t, errors.Is(err, ErrUpstreamResponseBodyTooLarge))
		require.True(t, called)
	})

	t.Run("nil onTooLarge does not panic", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Gateway.UpstreamResponseReadMaxBytes = 3

		body, err := ReadUpstreamResponseBody(bytes.NewReader([]byte("toolong")), cfg, nil, nil)
		require.Nil(t, body)
		require.True(t, errors.Is(err, ErrUpstreamResponseBodyTooLarge))
	})

	t.Run("io error does not call onTooLarge", func(t *testing.T) {
		called := false
		onTooLarge := func(_ *gin.Context) { called = true }

		body, err := ReadUpstreamResponseBody(iotest.ErrReader(errors.New("disk failure")), nil, nil, onTooLarge)
		require.Nil(t, body)
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrUpstreamResponseBodyTooLarge))
		require.False(t, called)
	})
}
