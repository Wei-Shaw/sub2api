package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	pluginManifestFilename                   = "manifest.json"
	pluginSignatureFilename                  = "signature.json"
	pluginArchiveMaxFiles                    = 512
	builtInOpenAITransportPluginID           = "local.sub2api.openai-transport"
	builtInOpenAITransportPublisherKeyID     = "sub2api-openai-transport-v1"
	builtInOpenAITransportPublisherKeyBase64 = "REDACTED"
)

type PluginPackageInstaller struct {
	cfg      *config.Config
	hostInfo PluginHostInfo
	rootDir  string
REDACTED

func NewPluginPackageInstaller(cfg *config.Config, hostInfo PluginHostInfo) *PluginPackageInstaller {
	return &PluginPackageInstaller{
		cfg:      cfg,
		hostInfo: hostInfo,
		rootDir:  resolvePluginRootDir(cfg),
REDACTED
REDACTED

func resolvePluginRootDir(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Plugins.DataDir) != "" {
		return filepath.Clean(cfg.Plugins.DataDir)
REDACTED
	base := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if base == "" {
		base = "./data"
REDACTED
	return filepath.Join(base, "plugins")
REDACTED

func (i *PluginPackageInstaller) RootDir() string {
	return i.rootDir
REDACTED

func (i *PluginPackageInstaller) Install(ctx context.Context, reader io.Reader, installedBy *int64) (*PluginInstallation, error) {
	if i == nil || i.cfg == nil {
		return nil, errors.New("插件安装器未配置")
REDACTED
	if err := ctx.Err(); err != nil {
		return nil, err
REDACTED
	stagingDir := filepath.Join(i.rootDir, "staging")
	packagesDir := filepath.Join(i.rootDir, "packages")
	installedDir := filepath.Join(i.rootDir, "installed")
	for _, dir := range []string{stagingDir, packagesDir, installedDirREDACTED {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("创建插件目录: %w", err)
	REDACTED
REDACTED

	tempFile, err := os.CreateTemp(stagingDir, "upload-*.s2plugin")
	if err != nil {
		return nil, fmt.Errorf("创建插件上传临时文件: %w", err)
REDACTED
	tempPath := tempFile.Name()
	committed := false
	defer func() {
		_ = tempFile.Close()
		if !committed {
			_ = os.Remove(tempPath)
	REDACTED
REDACTED()

	hasher := sha256.New()
	limit := i.cfg.Plugins.MaxUploadBytes
	written, err := io.Copy(io.MultiWriter(tempFile, hasher), io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("读取插件包: %w", err)
REDACTED
	if written > limit {
		return nil, fmt.Errorf("插件包超过 %d 字节限制", limit)
REDACTED
	if err := tempFile.Sync(); err != nil {
		return nil, fmt.Errorf("同步插件包: %w", err)
REDACTED
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("关闭插件包: %w", err)
REDACTED
	artifactSHA := hex.EncodeToString(hasher.Sum(nil))

	archive, err := zip.OpenReader(tempPath)
	if err != nil {
		return nil, fmt.Errorf("插件包不是有效的 ZIP: %w", err)
REDACTED
	defer func() { _ = archive.Close() REDACTED()
	manifest, _, signatureStatus, err := i.inspectArchive(&archive.Reader)
	if err != nil {
		return nil, err
REDACTED
	compatibility := EvaluatePluginCompatibility(manifest, i.hostInfo)
	initialState := PluginStateDisabled
	if !compatibility.Compatible {
		initialState = PluginStateIncompatible
REDACTED

	installParent := filepath.Join(installedDir, manifest.ID)
	if err := os.MkdirAll(installParent, 0o700); err != nil {
		return nil, fmt.Errorf("创建插件安装父目录: %w", err)
REDACTED
	extractPath, err := os.MkdirTemp(installParent, ".install-*")
	if err != nil {
		return nil, fmt.Errorf("创建插件安装临时目录: %w", err)
REDACTED
	installNonce := strings.TrimPrefix(filepath.Base(extractPath), ".install-")
	installPath := filepath.Join(installParent, manifest.Version+"-"+artifactSHA[:12]+"-"+installNonce)
	extracted := false
	defer func() {
		if !extracted {
			_ = os.RemoveAll(extractPath)
	REDACTED
REDACTED()
	if err := i.extractArchive(ctx, &archive.Reader, manifest, extractPath); err != nil {
		return nil, err
REDACTED
	if err := os.Rename(extractPath, installPath); err != nil {
		return nil, fmt.Errorf("提交插件安装目录: %w", err)
REDACTED
	extracted = true

	artifactPath := filepath.Join(packagesDir, manifest.ID+"-"+manifest.Version+"-"+artifactSHA[:12]+"-"+installNonce+".s2plugin")
	if err := os.Rename(tempPath, artifactPath); err != nil {
		_ = os.RemoveAll(installPath)
		return nil, fmt.Errorf("保存插件包: %w", err)
REDACTED
	committed = true
	artifactData, err := os.ReadFile(artifactPath)
	if err != nil {
		_ = os.Remove(artifactPath)
		_ = os.RemoveAll(installPath)
		return nil, fmt.Errorf("读取已保存插件包: %w", err)
REDACTED
	runtimeEntry := manifest.Runtimes[manifest.RuntimeKey()]
	return &PluginInstallation{
		PluginKey:       manifest.ID,
		Name:            manifest.Name,
		Version:         manifest.Version,
		Description:     manifest.Description,
		Author:          manifest.Author,
		Manifest:        manifest,
		ArtifactData:    artifactData,
		ArtifactPath:    artifactPath,
		InstallPath:     installPath,
		BinaryPath:      filepath.Join(installPath, filepath.FromSlash(runtimeEntry.Path)),
		BinarySHA256:    manifest.Files[runtimeEntry.Path],
		SignatureStatus: signatureStatus,
		State:           initialState,
		InstalledBy:     installedBy,
		Compatibility:   compatibility,
REDACTED, nil
REDACTED

func (i *PluginPackageInstaller) inspectArchive(archive *zip.Reader) (PluginManifest, []byte, string, error) {
	if len(archive.File) == 0 || len(archive.File) > pluginArchiveMaxFiles {
		return PluginManifest{REDACTED, nil, "", errors.New("插件包文件数量无效")
REDACTED
	entries := make(map[string]*zip.File, len(archive.File))
	var total uint64
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			if _, err := normalizePluginArchivePath(strings.TrimSuffix(file.Name, "/")); err != nil {
				return PluginManifest{REDACTED, nil, "", err
		REDACTED
			continue
	REDACTED
		name, err := normalizePluginArchivePath(file.Name)
		if err != nil {
			return PluginManifest{REDACTED, nil, "", err
	REDACTED
		if _, exists := entries[name]; exists {
			return PluginManifest{REDACTED, nil, "", fmt.Errorf("插件包包含重复路径: %s", name)
	REDACTED
		if file.Mode()&os.ModeSymlink != 0 {
			return PluginManifest{REDACTED, nil, "", fmt.Errorf("插件包不允许符号链接: %s", name)
	REDACTED
		total += file.UncompressedSize64
		if total > uint64(i.cfg.Plugins.MaxUncompressedBytes) {
			return PluginManifest{REDACTED, nil, "", errors.New("插件包解压后体积超过限制")
	REDACTED
		entries[name] = file
REDACTED
	manifestFile := entries[pluginManifestFilename]
	if manifestFile == nil {
		return PluginManifest{REDACTED, nil, "", errors.New("插件包缺少 manifest.json")
REDACTED
	manifestRaw, err := readPluginZipFile(manifestFile, 2*1024*1024)
	if err != nil {
		return PluginManifest{REDACTED, nil, "", fmt.Errorf("读取插件清单: %w", err)
REDACTED
	var manifest PluginManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return PluginManifest{REDACTED, nil, "", fmt.Errorf("解析插件清单: %w", err)
REDACTED
	if err := decoder.Decode(&struct{REDACTED{REDACTED); err != io.EOF {
		return PluginManifest{REDACTED, nil, "", errors.New("插件清单只能包含一个 JSON 对象")
REDACTED
	if err := manifest.Validate(); err != nil {
		return PluginManifest{REDACTED, nil, "", err
REDACTED
	for path := range entries {
		if path == pluginManifestFilename || path == pluginSignatureFilename {
			continue
	REDACTED
		if _, declared := manifest.Files[path]; !declared {
			return PluginManifest{REDACTED, nil, "", fmt.Errorf("插件包包含未声明文件: %s", path)
	REDACTED
REDACTED
	for path := range manifest.Files {
		if entries[path] == nil {
			return PluginManifest{REDACTED, nil, "", fmt.Errorf("插件包缺少已声明文件: %s", path)
	REDACTED
REDACTED
	signatureStatus, err := i.verifySignature(entries[pluginSignatureFilename], manifestRaw, manifest.ID)
	if err != nil {
		return PluginManifest{REDACTED, nil, "", err
REDACTED
	return manifest, manifestRaw, signatureStatus, nil
REDACTED

func (i *PluginPackageInstaller) verifySignature(file *zip.File, manifestRaw []byte, pluginID string) (string, error) {
	if file == nil {
		if i.cfg.Plugins.AllowUnsigned {
			return PluginSignatureUnsigned, nil
	REDACTED
		return "", errors.New("生产配置不允许安装未签名插件")
REDACTED
	raw, err := readPluginZipFile(file, 64*1024)
	if err != nil {
		return "", fmt.Errorf("读取插件签名: %w", err)
REDACTED
	var signature PluginSignature
	if err := json.Unmarshal(raw, &signature); err != nil {
		return "", fmt.Errorf("解析插件签名: %w", err)
REDACTED
	if signature.Algorithm != "ed25519" || strings.TrimSpace(signature.KeyID) == "" {
		return "", errors.New("插件签名算法或密钥 ID 无效")
REDACTED
	encodedKey := trustedPluginPublisherKey(i.cfg, signature.KeyID, pluginID)
	if encodedKey == "" {
		return "", fmt.Errorf("插件发布者密钥不受信任: %s", signature.KeyID)
REDACTED
	publicKey, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("受信任发布者密钥无效: %s", signature.KeyID)
REDACTED
	signatureBytes, err := base64.StdEncoding.DecodeString(signature.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), manifestRaw, signatureBytes) {
		return "", errors.New("插件签名校验失败")
REDACTED
	return PluginSignatureTrusted, nil
REDACTED

func trustedPluginPublisherKey(cfg *config.Config, keyID, pluginID string) string {
	// 内置公钥是官方私有插件的固定信任根，不允许被部署配置覆盖。
	if keyID == builtInOpenAITransportPublisherKeyID {
		if pluginID != builtInOpenAITransportPluginID {
			return ""
	REDACTED
		return builtInOpenAITransportPublisherKeyBase64
REDACTED
	if cfg == nil {
		return ""
REDACTED
	return strings.TrimSpace(cfg.Plugins.TrustedPublishers[keyID])
REDACTED

func (i *PluginPackageInstaller) extractArchive(ctx context.Context, archive *zip.Reader, manifest PluginManifest, target string) error {
	var extractedBytes int64
	extractLimit := i.cfg.Plugins.MaxUncompressedBytes
	for path, expectedHash := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
	REDACTED
		var source *zip.File
		for _, file := range archive.File {
			if strings.ReplaceAll(file.Name, "\\", "/") == path {
				source = file
				break
		REDACTED
	REDACTED
		if source == nil {
			return fmt.Errorf("缺少插件文件: %s", path)
	REDACTED
		destination, err := safePluginJoin(target, path)
		if err != nil {
			return err
	REDACTED
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return fmt.Errorf("创建插件文件目录: %w", err)
	REDACTED
		input, err := source.Open()
		if err != nil {
			return fmt.Errorf("打开插件文件 %s: %w", path, err)
	REDACTED
		hasher := sha256.New()
		mode := os.FileMode(0o600)
		if path == manifest.Runtimes[manifest.RuntimeKey()].Path {
			mode = 0o700
	REDACTED
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			_ = input.Close()
			return fmt.Errorf("创建插件文件 %s: %w", path, err)
	REDACTED
		remaining := extractLimit - extractedBytes
		if remaining < 0 {
			remaining = 0
	REDACTED
		copied, copyErr := io.Copy(io.MultiWriter(output, hasher), io.LimitReader(input, remaining+1))
		extractedBytes += copied
		closeOutErr := output.Close()
		closeInErr := input.Close()
		if copyErr != nil || closeOutErr != nil || closeInErr != nil {
			return fmt.Errorf("解压插件文件 %s 失败", path)
	REDACTED
		if extractedBytes > extractLimit {
			return errors.New("插件包实际解压体积超过限制")
	REDACTED
		if actual := hex.EncodeToString(hasher.Sum(nil)); actual != expectedHash {
			return fmt.Errorf("插件文件哈希不匹配: %s", path)
	REDACTED
REDACTED
	return nil
REDACTED

func normalizePluginArchivePath(name string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") || strings.Contains(normalized, "\x00") {
		return "", errors.New("插件包包含无效路径")
REDACTED
	cleaned := filepath.ToSlash(filepath.Clean(normalized))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != normalized {
		return "", fmt.Errorf("插件包包含不安全路径: %s", name)
REDACTED
	return cleaned, nil
REDACTED

func safePluginJoin(root, relative string) (string, error) {
	destination := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, destination)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("插件路径越界: %s", relative)
REDACTED
	return destination, nil
REDACTED

func readPluginZipFile(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = reader.Close() REDACTED()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
REDACTED
	if int64(len(data)) > limit {
		return nil, errors.New("插件文件超过读取限制")
REDACTED
	return data, nil
REDACTED
