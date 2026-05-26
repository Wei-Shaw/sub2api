package logger

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestResolveLogFilePath_Default(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	got := resolveLogFilePath("")
	if got != DefaultContainerLogPath {
		t.Fatalf("resolveLogFilePath() = %q, want %q", got, DefaultContainerLogPath)
	}
}

func TestResolveLogFilePath_WithDataDir(t *testing.T) {
	t.Setenv("DATA_DIR", "/tmp/sub2api-data")
	got := resolveLogFilePath("")
	want := filepath.Join("/tmp/sub2api-data", "logs", "sub2api.log")
	if got != want {
		t.Fatalf("resolveLogFilePath() = %q, want %q", got, want)
	}
}

func TestResolveLogFilePath_ExplicitPath(t *testing.T) {
	t.Setenv("DATA_DIR", "/tmp/ignore")
	got := resolveLogFilePath("/var/log/custom.log")
	if got != "/var/log/custom.log" {
		t.Fatalf("resolveLogFilePath() = %q, want explicit path", got)
	}
}

func TestNormalizedOptions_InvalidFallback(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	opts := InitOptions{
		Level:           "TRACE",
		Format:          "TEXT",
		ServiceName:     "",
		Environment:     "",
		StacktraceLevel: "panic",
		Output: OutputOptions{
			ToStdout: false,
			ToFile:   false,
		},
		Rotation: RotationOptions{
			MaxSizeMB:  0,
			MaxBackups: -1,
			MaxAgeDays: -1,
		},
		Sampling: SamplingOptions{
			Enabled:    true,
			Initial:    0,
			Thereafter: 0,
		},
	}
	out := opts.normalized()
	if out.Level != "trace" {
		// normalized 仅做 trim/lower，不做校验；校验在 config 层。
		t.Fatalf("normalized level should preserve value for upstream validation, got %q", out.Level)
	}
	if !out.Output.ToStdout {
		t.Fatalf("normalized output should fallback to stdout")
	}
	if out.Output.FilePath != DefaultContainerLogPath {
		t.Fatalf("normalized file path = %q", out.Output.FilePath)
	}
	if out.Rotation.MaxSizeMB != 100 {
		t.Fatalf("normalized max_size_mb = %d", out.Rotation.MaxSizeMB)
	}
	if out.Rotation.MaxBackups != 10 {
		t.Fatalf("normalized max_backups = %d", out.Rotation.MaxBackups)
	}
	if out.Rotation.MaxAgeDays != 7 {
		t.Fatalf("normalized max_age_days = %d", out.Rotation.MaxAgeDays)
	}
	if out.Sampling.Initial != 100 || out.Sampling.Thereafter != 100 {
		t.Fatalf("normalized sampling defaults invalid: %+v", out.Sampling)
	}
}

func TestBuildFileCore_InvalidPathFallback(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	opts := bootstrapOptions()
	opts.Output.ToFile = true
	opts.Output.FilePath = filepath.Join(os.DevNull, "logs", "sub2api.log")
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:     "time",
		LevelKey:    "level",
		MessageKey:  "msg",
		EncodeTime:  zapcore.ISO8601TimeEncoder,
		EncodeLevel: zapcore.CapitalLevelEncoder,
	}
	encoder := zapcore.NewJSONEncoder(encoderCfg)
	_, _, err := buildFileCore(encoder, zap.NewAtomicLevel(), opts)
	if err == nil {
		t.Fatalf("buildFileCore() expected error for invalid path")
	}
}
