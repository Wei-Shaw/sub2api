package setup

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRedisSetupUnixValidation(t *testing.T) {
	for _, tc := range []struct {
		name, host   string
		port         int
		tls, wantErr bool
	}{
		{"short", "unix:/tmp/redis.sock", 0, false, false},
		{"triple slash", "unix:///tmp/redis.sock", 0, false, false},
		{"nonzero port", "unix:/tmp/redis.sock", 6379, false, true},
		{"tls", "unix:/tmp/redis.sock", 0, true, true},
		{"relative", "unix:redis.sock", 0, false, true},
		{"tcp", "localhost", 6379, false, false},
		{"tcp tls", "localhost", 6379, true, false},
		{"tcp missing port", "localhost", 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRedisConnection(tc.host, tc.port, tc.tls)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	// A zero/omitted/null UDS port must pass JSON binding before validation.
	for _, body := range []string{
		`{"host":"unix:/tmp/redis.sock"}`,
		`{"host":"unix:/tmp/redis.sock","port":0}`,
		`{"host":"unix:/tmp/redis.sock","port":null}`,
	} {
		var req TestRedisRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		require.NoError(t, binding.Validator.ValidateStruct(&req))
		require.NoError(t, validateRedisConnection(req.Host, req.Port, req.EnableTLS))
	}
}

func TestRedisSetupUnixConfigPersistence(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	require.NoError(t, writeConfigFile(&SetupConfig{Redis: RedisConfig{Host: "unix:/tmp/redis.sock", Password: "test", DB: 2}}))
	data, err := os.ReadFile(GetConfigFilePath())
	require.NoError(t, err)
	var saved struct {
		Redis RedisConfig `yaml:"redis"`
	}
	require.NoError(t, yaml.Unmarshal(data, &saved))
	require.Equal(t, "unix:/tmp/redis.sock", saved.Redis.Host)
	require.Zero(t, saved.Redis.Port)
	require.False(t, saved.Redis.EnableTLS)
	require.Equal(t, "test", saved.Redis.Password)
	require.Equal(t, 2, saved.Redis.DB)
	rc := config.RedisConfig{Host: saved.Redis.Host, Port: saved.Redis.Port, EnableTLS: saved.Redis.EnableTLS}
	require.NoError(t, rc.ValidateUnix())
}
