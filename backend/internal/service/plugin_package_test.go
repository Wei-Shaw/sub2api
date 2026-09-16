package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginPackageInstallerInstallUnsignedDevelopmentPackage(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179", BuildType: "release"})
	archive := buildTestPluginArchive(t, nil, "")

	installation, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)

	require.NoError(t, err)
	assert.Equal(t, PluginStateDisabled, installation.State)
	assert.Equal(t, PluginSignatureUnsigned, installation.SignatureStatus)
	assert.FileExists(t, installation.ArtifactPath)
	info, statErr := os.Stat(installation.BinaryPath)
	require.NoError(t, statErr)
	// Windows 没有 POSIX 执行位（FileMode 仅映射只读属性），该断言仅在类 Unix 平台有意义
	if runtime.GOOS != "windows" {
		assert.NotZero(t, info.Mode()&0o100)
	}
	assert.Contains(t, installation.InstallPath, filepath.Join("installed", "com.example.openai-transport"))
}

func TestPluginPackageInstallerAllowsRepeatedIdenticalUpload(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"})
	archive := buildTestPluginArchive(t, nil, "")

	first, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)
	require.NoError(t, err)
	second, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)
	require.NoError(t, err)

	assert.NotEqual(t, first.InstallPath, second.InstallPath)
	assert.NotEqual(t, first.ArtifactPath, second.ArtifactPath)
	assert.FileExists(t, first.BinaryPath)
	assert.FileExists(t, second.BinaryPath)
}

func TestPluginPackageInstallerRejectsUnsignedPackageByDefault(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), false)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"})

	_, err := installer.Install(context.Background(), bytes.NewReader(buildTestPluginArchive(t, nil, "")), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "不允许安装未签名插件")
}

func TestPluginPackageInstallerVerifiesTrustedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg := testPluginConfig(t.TempDir(), false)
	cfg.Plugins.TrustedPublishers["local-test"] = base64.StdEncoding.EncodeToString(publicKey)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"})

	installation, installErr := installer.Install(
		context.Background(),
		bytes.NewReader(buildTestPluginArchive(t, privateKey, "local-test")),
		nil,
	)

	require.NoError(t, installErr)
	assert.Equal(t, PluginSignatureTrusted, installation.SignatureStatus)
}

func TestBuiltInOpenAITransportPublisherDoesNotRequireConfiguration(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), false)
	cfg.Plugins.TrustedPublishers[builtInOpenAITransportPublisherKeyID] = "不能覆盖内置公钥"

	encodedKey := trustedPluginPublisherKey(cfg, builtInOpenAITransportPublisherKeyID, builtInOpenAITransportPluginID)
	publicKey, err := base64.StdEncoding.DecodeString(encodedKey)

	require.NoError(t, err)
	assert.Len(t, publicKey, ed25519.PublicKeySize)
	assert.Equal(t, builtInOpenAITransportPublisherKeyBase64, encodedKey)
	assert.Empty(t, trustedPluginPublisherKey(cfg, builtInOpenAITransportPublisherKeyID, "com.example.other-plugin"))
}

func TestPluginPackageInstallerRejectsPathTraversal(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"})
	archive := buildTestPluginArchiveWithExtra(t, nil, "", "../escape", []byte("escape"))

	_, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "不安全路径")
}

func TestPluginPackageInstallerKeepsHostVersionMismatchDisabled(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.2.0"})

	installation, err := installer.Install(context.Background(), bytes.NewReader(buildTestPluginArchive(t, nil, "")), nil)

	require.NoError(t, err)
	assert.Equal(t, PluginStateIncompatible, installation.State)
	assert.False(t, installation.Compatibility.Compatible)
}

func TestPluginPackageInstallerRejectsHashMismatch(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"})
	manifest := testPluginManifest(map[string][]byte{
		"bin/plugin":    []byte("binary"),
		"ui/index.html": []byte("<html></html>"),
	})
	manifest.Files["bin/plugin"] = string(bytes.Repeat([]byte("0"), 64))

	_, err := installer.Install(context.Background(), bytes.NewReader(buildPluginArchive(t, manifest, nil, "", nil)), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "哈希不匹配")
}

func TestPluginPackageInstallerEnforcesActualExtractionLimit(t *testing.T) {
	archiveData := buildTestPluginArchive(t, nil, "")
	archive, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	require.NoError(t, err)
	cfg := testPluginConfig(t.TempDir(), true)
	cfg.Plugins.MaxUncompressedBytes = 8
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"})

	err = installer.extractArchive(context.Background(), archive, testPluginManifest(nil), t.TempDir())

	require.ErrorContains(t, err, "实际解压体积超过限制")
}

func testPluginConfig(root string, allowUnsigned bool) *config.Config {
	return &config.Config{Plugins: config.PluginConfig{
		DataDir:              root,
		AllowUnsigned:        allowUnsigned,
		TrustedPublishers:    map[string]string{},
		MaxUploadBytes:       64 * 1024 * 1024,
		MaxUncompressedBytes: 128 * 1024 * 1024,
		StartTimeoutSeconds:  5,
	}}
}

func testPluginManifest(files map[string][]byte) PluginManifest {
	if files == nil {
		files = map[string][]byte{
			"bin/plugin":    []byte("binary"),
			"ui/index.html": []byte("<html></html>"),
		}
	}
	hashes := make(map[string]string, len(files))
	for path, data := range files {
		digest := sha256.Sum256(data)
		hashes[path] = hex.EncodeToString(digest[:])
	}
	return PluginManifest{
		SchemaVersion: 1,
		ID:            "com.example.openai-transport",
		Name:          "测试 OpenAI Transport",
		Version:       "0.1.0",
		Requires: PluginRequirements{
			Sub2API:                   ">=0.1.170 <0.2.0",
			RecommendedSub2APIVersion: "0.1.179",
			TestedSub2APIVersions:     []string{"0.1.179"},
			PluginProtocol:            pluginv1.ProtocolVersion,
			TransportAPI:              pluginv1.TransportAPIVersion,
			UIBridge:                  pluginv1.UIBridgeVersion,
		},
		Capabilities: []PluginCapability{{
			ID:          PluginCapabilityOpenAIOAuthOutbound,
			Platform:    PlatformOpenAI,
			AccountType: AccountTypeOAuth,
		}},
		Runtimes: map[string]PluginRuntime{
			PluginManifest{}.RuntimeKey(): {Path: "bin/plugin"},
		},
		UI:    PluginUIManifest{Entrypoint: "ui/index.html"},
		Files: hashes,
	}
}

func buildTestPluginArchive(t *testing.T, privateKey ed25519.PrivateKey, keyID string) []byte {
	return buildPluginArchive(t, testPluginManifest(nil), privateKey, keyID, nil)
}

func buildTestPluginArchiveWithExtra(t *testing.T, privateKey ed25519.PrivateKey, keyID, path string, data []byte) []byte {
	return buildPluginArchive(t, testPluginManifest(nil), privateKey, keyID, map[string][]byte{path: data})
}

func buildPluginArchive(
	t *testing.T,
	manifest PluginManifest,
	privateKey ed25519.PrivateKey,
	keyID string,
	extra map[string][]byte,
) []byte {
	t.Helper()
	manifestRaw, err := json.Marshal(manifest)
	require.NoError(t, err)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	writeZipEntry(t, writer, pluginManifestFilename, manifestRaw)
	for path := range manifest.Files {
		data := []byte("binary")
		if path == "ui/index.html" {
			data = []byte("<html></html>")
		}
		writeZipEntry(t, writer, path, data)
	}
	for path, data := range extra {
		writeZipEntry(t, writer, path, data)
	}
	if len(privateKey) > 0 {
		signatureRaw, marshalErr := json.Marshal(PluginSignature{
			Algorithm: "ed25519",
			KeyID:     keyID,
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifestRaw)),
		})
		require.NoError(t, marshalErr)
		writeZipEntry(t, writer, pluginSignatureFilename, signatureRaw)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func writeZipEntry(t *testing.T, writer *zip.Writer, path string, data []byte) {
	t.Helper()
	entry, err := writer.Create(path)
	require.NoError(t, err)
	_, err = entry.Write(data)
	require.NoError(t, err)
}
