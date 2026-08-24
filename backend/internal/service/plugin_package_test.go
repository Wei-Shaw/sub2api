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
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginPackageInstallerInstallUnsignedDevelopmentPackage(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179", BuildType: "release"REDACTED)
	archive := buildTestPluginArchive(t, nil, "")

	installation, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)

REDACTED
	assert.Equal(t, PluginStateDisabled, installation.State)
	assert.Equal(t, PluginSignatureUnsigned, installation.SignatureStatus)
	assert.FileExists(t, installation.ArtifactPath)
	info, statErr := os.Stat(installation.BinaryPath)
	require.NoError(t, statErr)
	assert.NotZero(t, info.Mode()&0o100)
	assert.Contains(t, installation.InstallPath, filepath.Join("installed", "com.example.openai-transport"))
REDACTED

func TestPluginPackageInstallerAllowsRepeatedIdenticalUpload(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"REDACTED)
	archive := buildTestPluginArchive(t, nil, "")

	first, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)
REDACTED
	second, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)
REDACTED

	assert.NotEqual(t, first.InstallPath, second.InstallPath)
	assert.NotEqual(t, first.ArtifactPath, second.ArtifactPath)
	assert.FileExists(t, first.BinaryPath)
	assert.FileExists(t, second.BinaryPath)
REDACTED

func TestPluginPackageInstallerRejectsUnsignedPackageByDefault(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), false)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"REDACTED)

	_, err := installer.Install(context.Background(), bytes.NewReader(buildTestPluginArchive(t, nil, "")), nil)

REDACTED
	assert.Contains(t, err.Error(), "不允许安装未签名插件")
REDACTED

func TestPluginPackageInstallerVerifiesTrustedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
REDACTED
	cfg := testPluginConfig(t.TempDir(), false)
	cfg.Plugins.TrustedPublishers["local-test"] = base64.StdEncoding.EncodeToString(publicKey)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"REDACTED)

	installation, installErr := installer.Install(
		context.Background(),
		bytes.NewReader(buildTestPluginArchive(t, privateKey, "local-test")),
		nil,
	)

	require.NoError(t, installErr)
	assert.Equal(t, PluginSignatureTrusted, installation.SignatureStatus)
REDACTED

func TestBuiltInOpenAITransportPublisherDoesNotRequireConfiguration(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), false)
	cfg.Plugins.TrustedPublishers[builtInOpenAITransportPublisherKeyID] = "不能覆盖内置公钥"

	encodedKey := trustedPluginPublisherKey(cfg, builtInOpenAITransportPublisherKeyID, builtInOpenAITransportPluginID)
	publicKey, err := base64.StdEncoding.DecodeString(encodedKey)

REDACTED
	assert.Len(t, publicKey, ed25519.PublicKeySize)
	assert.Equal(t, builtInOpenAITransportPublisherKeyBase64, encodedKey)
	assert.Empty(t, trustedPluginPublisherKey(cfg, builtInOpenAITransportPublisherKeyID, "com.example.other-plugin"))
REDACTED

func TestPluginPackageInstallerRejectsPathTraversal(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"REDACTED)
	archive := buildTestPluginArchiveWithExtra(t, nil, "", "../escape", []byte("escape"))

	_, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)

REDACTED
	assert.Contains(t, err.Error(), "不安全路径")
REDACTED

func TestPluginPackageInstallerKeepsHostVersionMismatchDisabled(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.2.0"REDACTED)

	installation, err := installer.Install(context.Background(), bytes.NewReader(buildTestPluginArchive(t, nil, "")), nil)

REDACTED
	assert.Equal(t, PluginStateIncompatible, installation.State)
	assert.False(t, installation.Compatibility.Compatible)
REDACTED

func TestPluginPackageInstallerRejectsHashMismatch(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), true)
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"REDACTED)
	manifest := testPluginManifest(map[string][]byte{
		"bin/plugin":    []byte("binary"),
		"ui/index.html": []byte("<html></html>"),
REDACTED)
	manifest.Files["bin/plugin"] = string(bytes.Repeat([]byte("0"), 64))

	_, err := installer.Install(context.Background(), bytes.NewReader(buildPluginArchive(t, manifest, nil, "", nil)), nil)

REDACTED
	assert.Contains(t, err.Error(), "哈希不匹配")
REDACTED

func TestPluginPackageInstallerEnforcesActualExtractionLimit(t *testing.T) {
	archiveData := buildTestPluginArchive(t, nil, "")
	archive, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
REDACTED
	cfg := testPluginConfig(t.TempDir(), true)
	cfg.Plugins.MaxUncompressedBytes = 8
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "0.1.179"REDACTED)

	err = installer.extractArchive(context.Background(), archive, testPluginManifest(nil), t.TempDir())

	require.ErrorContains(t, err, "实际解压体积超过限制")
REDACTED

func testPluginConfig(root string, allowUnsigned bool) *config.Config {
	return &config.Config{Plugins: config.PluginConfig{
		DataDir:              root,
		AllowUnsigned:        allowUnsigned,
		TrustedPublishers:    map[string]string{REDACTED,
		MaxUploadBytes:       64 * 1024 * 1024,
		MaxUncompressedBytes: 128 * 1024 * 1024,
		StartTimeoutSeconds:  5,
REDACTEDREDACTED
REDACTED

func testPluginManifest(files map[string][]byte) PluginManifest {
	if files == nil {
		files = map[string][]byte{
			"bin/plugin":    []byte("binary"),
			"ui/index.html": []byte("<html></html>"),
	REDACTED
REDACTED
	hashes := make(map[string]string, len(files))
	for path, data := range files {
		digest := sha256.Sum256(data)
		hashes[path] = hex.EncodeToString(digest[:])
REDACTED
	return PluginManifest{
		SchemaVersion: 1,
		ID:            "com.example.openai-transport",
		Name:          "测试 OpenAI Transport",
		Version:       "0.1.0",
		Requires: PluginRequirements{
			Sub2API:                   ">=0.1.170 <0.2.0",
			RecommendedSub2APIVersion: "0.1.179",
			TestedSub2APIVersions:     []string{"0.1.179"REDACTED,
			PluginProtocol:            pluginv1.ProtocolVersion,
			TransportAPI:              pluginv1.TransportAPIVersion,
			UIBridge:                  pluginv1.UIBridgeVersion,
	REDACTED,
		Capabilities: []PluginCapability{{
			ID:          PluginCapabilityOpenAIOAuthOutbound,
			Platform:    PlatformOpenAI,
			AccountType: AccountTypeOAuth,
REDACTED
		Runtimes: map[string]PluginRuntime{
			PluginManifest{REDACTED.RuntimeKey(): {Path: "bin/plugin"REDACTED,
	REDACTED,
		UI:    PluginUIManifest{Entrypoint: "ui/index.html"REDACTED,
		Files: hashes,
REDACTED
REDACTED

func buildTestPluginArchive(t *testing.T, privateKey ed25519.PrivateKey, keyID string) []byte {
	return buildPluginArchive(t, testPluginManifest(nil), privateKey, keyID, nil)
REDACTED

func buildTestPluginArchiveWithExtra(t *testing.T, privateKey ed25519.PrivateKey, keyID, path string, data []byte) []byte {
	return buildPluginArchive(t, testPluginManifest(nil), privateKey, keyID, map[string][]byte{path: dataREDACTED)
REDACTED

func buildPluginArchive(
	t *testing.T,
	manifest PluginManifest,
	privateKey ed25519.PrivateKey,
	keyID string,
	extra map[string][]byte,
) []byte {
REDACTED
	manifestRaw, err := json.Marshal(manifest)
REDACTED
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	writeZipEntry(t, writer, pluginManifestFilename, manifestRaw)
	for path := range manifest.Files {
		data := []byte("binary")
		if path == "ui/index.html" {
			data = []byte("<html></html>")
	REDACTED
		writeZipEntry(t, writer, path, data)
REDACTED
	for path, data := range extra {
		writeZipEntry(t, writer, path, data)
REDACTED
	if len(privateKey) > 0 {
		signatureRaw, marshalErr := json.Marshal(PluginSignature{
			Algorithm: "ed25519",
			KeyID:     keyID,
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifestRaw)),
	REDACTED)
		require.NoError(t, marshalErr)
		writeZipEntry(t, writer, pluginSignatureFilename, signatureRaw)
REDACTED
	require.NoError(t, writer.Close())
	return buffer.Bytes()
REDACTED

func writeZipEntry(t *testing.T, writer *zip.Writer, path string, data []byte) {
REDACTED
	entry, err := writer.Create(path)
REDACTED
	_, err = entry.Write(data)
REDACTED
REDACTED
