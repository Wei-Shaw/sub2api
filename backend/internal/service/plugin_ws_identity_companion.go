package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
)

type wsIdentityCompanion struct {
	runtime *pluginRuntime
	epoch   uint64
	cleanup func()
	once    sync.Once
}

func (c *wsIdentityCompanion) close() {
	c.once.Do(func() {
		c.runtime.kill()
		c.cleanup()
	})
}

// nativeWSIdentity pins the optional client to the HTTP runtime config epoch.
// An unavailable or stale companion must not select a different native identity.
func (r *pluginRuntime) nativeWSIdentity(companionOnly bool) (pluginv1.WSIdentityPluginClient, uint64) {
	epoch := r.wsConfigEpoch.Load()
	if epoch%2 != 0 || r.draining.Load() {
		return nil, 0
	}
	if !companionOnly {
		return r.wsIdentity, epoch
	}
	c := r.wsCompanion
	if c == nil || c.epoch != epoch || c.runtime.client.Exited() {
		return nil, 0
	}
	return c.runtime.wsIdentity, epoch
}

// prepareWSIdentityCompanion only extracts a digest-pinned archive into a
// private directory owned by the running HTTP plugin. It never calls the
// plugin repository or grants host services to the companion process.
func (m *PluginManager) prepareWSIdentityCompanion(ctx context.Context, httpRuntime *pluginRuntime, raw, normalized []byte) (*wsIdentityCompanion, error) {
	if httpRuntime.installation == nil || httpRuntime.installation.SignatureStatus != PluginSignatureTrusted {
		return nil, errors.New("HTTP plugin must have a trusted signature")
	}
	cfg := *m.cfg
	archive, err := readWSIdentityCompanionArchive(cfg.Plugins.OpenAIWSIdentityCompanionPackage, cfg.Plugins.OpenAIWSIdentityCompanionSHA256, cfg.Plugins.MaxUploadBytes)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(m.installer.RootDir(), ".ws-companion-")
	if err != nil {
		return nil, errors.New("could not allocate companion directory")
	}
	cleanup := func() {
		if err := m.removeManagedPath(root); err != nil {
			slog.Warn("ws_identity_companion_cleanup_failed")
		}
	}
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	cfg.Plugins.AllowUnsigned = false
	cfg.Plugins.DataDir = root
	installer := NewPluginPackageInstaller(&cfg, m.hostInfo)
	installation, err := installer.Install(ctx, bytes.NewReader(archive), nil)
	if err != nil {
		return nil, errors.New("could not verify and extract signed companion archive")
	}
	if installation.SignatureStatus != PluginSignatureTrusted || installation.PluginKey != httpRuntime.installation.PluginKey ||
		!installation.Compatibility.Compatible || !pluginDeclaresWSIdentity(installation.Manifest) {
		return nil, errors.New("companion must be trusted, compatible, and declare the same plugin ID plus native WS")
	}
	timeout := time.Duration(cfg.Plugins.StartTimeoutSeconds) * time.Second
	runtime, err := startPluginRuntime(ctx, installation, timeout, root, nil)
	if err != nil {
		return nil, errors.New("companion runtime failed its handshake")
	}
	defer func() {
		if !keep {
			runtime.kill()
		}
	}()
	if runtime.wsIdentity == nil {
		return nil, errors.New("companion does not serve the native WS RPC")
	}
	applyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	applied, err := runtime.validateAndApplyNormalizedConfig(applyCtx, raw)
	if err != nil || !bytes.Equal(applied, normalized) {
		return nil, errors.New("companion did not accept the exact active HTTP plugin config")
	}
	if err := runtime.checkHealth(applyCtx); err != nil {
		return nil, errors.New("companion failed its post-configuration health check")
	}
	keep = true
	return &wsIdentityCompanion{runtime: runtime, epoch: httpRuntime.wsConfigEpoch.Load(), cleanup: cleanup}, nil
}

func readWSIdentityCompanionArchive(path, digest string, limit int64) ([]byte, error) {
	before, err := os.Stat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, errors.New("companion archive is not a regular file within the size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("companion archive is not readable")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || !os.SameFile(before, info) {
		return nil, errors.New("companion archive changed before it could be read")
	}
	archive, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(archive)) > limit {
		return nil, errors.New("could not read bounded companion archive")
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("companion archive SHA-256 does not match")
	}
	return archive, nil
}
