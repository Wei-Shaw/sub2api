package repository

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/redis/go-redis/v9"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildRedisOptions(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Host:                "localhost",
			Port:                6379,
			Username:            "app-user",
			Password:            "secret",
			DB:                  2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 4,
			PoolSize:            100,
			MinIdleConns:        10,
		},
	}

	opts := buildRedisOptions(cfg)
	require.Equal(t, "localhost:6379", opts.Addr)
	require.Equal(t, "app-user", opts.Username)
	require.Equal(t, "secret", opts.Password)
	require.Equal(t, 2, opts.DB)
	require.Equal(t, 5*time.Second, opts.DialTimeout)
	require.Equal(t, 3*time.Second, opts.ReadTimeout)
	require.Equal(t, 4*time.Second, opts.WriteTimeout)
	require.Equal(t, 100, opts.PoolSize)
	require.Equal(t, 10, opts.MinIdleConns)
	require.Nil(t, opts.TLSConfig)

	// Test case with TLS enabled
	cfgTLS := &config.Config{
		Redis: config.RedisConfig{
			Host:      "localhost",
			EnableTLS: true,
		},
	}
	optsTLS := buildRedisOptions(cfgTLS)
	require.NotNil(t, optsTLS.TLSConfig)
	require.Equal(t, "localhost", optsTLS.TLSConfig.ServerName)
}

func TestBuildRedisUnixOptions(t *testing.T) {
	for _, host := range []string{"unix:/tmp/redis.sock", "unix:///tmp/redis.sock"} {
		cfg := &config.Config{Redis: config.RedisConfig{Host: host, Username: "app", Password: "secret", DB: 2, PoolSize: 4, DialTimeoutSeconds: 1, ReadTimeoutSeconds: 2, WriteTimeoutSeconds: 3}}
		require.NoError(t, cfg.Redis.ValidateUnix())
		opts := buildRedisOptions(cfg)
		require.Equal(t, "unix", opts.Network)
		require.Equal(t, "/tmp/redis.sock", opts.Addr)
		require.Equal(t, "app", opts.Username)
		require.Equal(t, "secret", opts.Password)
		require.Equal(t, 2, opts.DB)
		require.Equal(t, 4, opts.PoolSize)
		require.Equal(t, time.Second, opts.DialTimeout)
		require.Equal(t, 2*time.Second, opts.ReadTimeout)
		require.Equal(t, 3*time.Second, opts.WriteTimeout)
		require.Nil(t, opts.TLSConfig)
	}
}

// Optional local integration test; never connects to the deployment's Redis.
func TestRedisUnixRoundTrip(t *testing.T) {
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is not installed")
	}
	dir, err := os.MkdirTemp("", "redis-uds-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "redis.sock")
	cmd := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--requirepass", "uds-test", "--maxmemory", "16mb")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := &config.Config{Redis: config.RedisConfig{Host: "unix:" + socket, Password: "uds-test", DB: 2, PoolSize: 1, DialTimeoutSeconds: 1, ReadTimeoutSeconds: 1, WriteTimeoutSeconds: 1}}
	client := InitRedis(cfg)
	defer func() { _ = client.Close() }()
	for client.Ping(ctx).Err() != nil {
		require.NoError(t, ctx.Err(), "Redis UDS did not become ready")
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(t, client.Set(ctx, "uds-check", "ok", time.Minute).Err())
	require.Equal(t, "ok", client.Get(ctx, "uds-check").Val())
	cfg.Redis.Host = "unix://" + socket
	cfg.Redis.DB = 0
	other := InitRedis(cfg)
	defer func() { _ = other.Close() }()
	require.NoError(t, other.Ping(ctx).Err())
	require.ErrorIs(t, other.Get(ctx, "uds-check").Err(), redis.Nil)
	cfg.Redis.Password = "wrong"
	denied := InitRedis(cfg)
	defer func() { _ = denied.Close() }()
	require.Error(t, denied.Ping(ctx).Err())
}
