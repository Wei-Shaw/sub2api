package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedisUnixValidation(t *testing.T) {
	for _, tc := range []struct {
		name, host string
		port       int
		tls        bool
		wantErr    bool
	}{
		{"short", "unix:/tmp/redis.sock", 0, false, false},
		{"triple slash", "unix:///tmp/redis.sock", 0, false, false},
		{"port", "unix:/tmp/redis.sock", 6379, false, true},
		{"tls", "unix:/tmp/redis.sock", 0, true, true},
		{"relative", "unix:redis.sock", 0, false, true},
		{"empty", "unix:", 0, false, true},
		{"authority", "unix://tmp/redis.sock", 0, false, true},
		{"credentials", "unix://user:password@/tmp/redis.sock", 0, false, true},
		{"query", "unix:/tmp/redis.sock?db=1", 0, false, true},
		{"empty query", "unix:/tmp/redis.sock?", 0, false, true},
		{"fragment", "unix:/tmp/redis.sock#x", 0, false, true},
		{"invalid escape", "unix:/tmp/%xx", 0, false, true},
		{"nul", "unix:/tmp/%00.sock", 0, false, true},
		{"tcp unchanged", "localhost", 6379, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := RedisConfig{Host: tc.host, Port: tc.port, EnableTLS: tc.tls}
			if tc.wantErr {
				require.Error(t, cfg.ValidateUnix())
				return
			}
			require.NoError(t, cfg.ValidateUnix())
			if cfg.IsUnix() {
				require.Equal(t, "/tmp/redis.sock", cfg.Address())
			} else {
				require.Equal(t, "localhost:6379", cfg.Address())
			}
		})
	}
}

func TestLoadRedisUnixPortDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, host, portYAML, envHost, envPort string
		wantPort                               int
		wantErr                                bool
	}{
		{name: "omitted", host: "unix:/tmp/redis.sock"},
		{name: "null", host: "unix:/tmp/redis.sock", portYAML: "  port:\n"},
		{name: "empty string", host: "unix:/tmp/redis.sock", portYAML: "  port: \"\"\n"},
		{name: "zero", host: "unix:///tmp/redis.sock", portYAML: "  port: 0\n"},
		{name: "nonzero", host: "unix:/tmp/redis.sock", portYAML: "  port: 6379\n", wantErr: true},
		{name: "environment zero overrides yaml", host: "unix:/tmp/redis.sock", portYAML: "  port: 6379\n", envPort: "0"},
		{name: "environment nonzero", host: "unix:/tmp/redis.sock", envPort: "6379", wantErr: true},
		{name: "environment host", host: "localhost", envHost: "unix:/tmp/redis.sock"},
		{name: "tcp default", host: "localhost", wantPort: 6379},
		{name: "tcp custom", host: "localhost", portYAML: "  port: 6380\n", wantPort: 6380},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("REDIS_HOST", tc.envHost)
			t.Setenv("REDIS_PORT", tc.envPort)
			t.Setenv("REDIS_ENABLE_TLS", "false")
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte("redis:\n  host: "+tc.host+"\n"+tc.portYAML), 0600))
			t.Setenv("CONFIG_FILE", path)
			cfg, err := Load()
			if tc.wantErr {
				require.ErrorContains(t, err, "require port")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantPort, cfg.Redis.Port)
			require.Equal(t, 0, cfg.Redis.DB)
		})
	}
}
