package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
)

func TestPluginCatalogSignedPackageRuntime(t *testing.T) {
	path := os.Getenv("SUB2API_TEST_CATALOG_PACKAGE")
	root := t.TempDir()
	cfg := testPluginConfig(root, false)
	var archive []byte
	if path != "" {
		var err error
		archive, err = os.ReadFile(path)
		require.NoError(t, err)
	} else if binaryPath := os.Getenv("SUB2API_TEST_CATALOG_BINARY"); binaryPath != "" {
		// Exercise the rebuilt plugin without reading a production signing key.
		binary, err := os.ReadFile(binaryPath)
		require.NoError(t, err)
		runtimePath := "bin/" + filepath.Base(binaryPath)
		files := map[string][]byte{runtimePath: binary}
		for _, name := range []string{"index.html", "app.js", "styles.css", "base-model.js", "catalog-models.js"} {
			files["ui/"+name], err = os.ReadFile(filepath.Join("..", "..", "plugins", "model-catalog", "ui", name))
			require.NoError(t, err)
		}
		manifest := testPluginManifest(files)
		manifest.ID = "local.sub2api.model-catalog"
		manifest.Version = "1.3.0"
		manifest.Requires.Sub2API = ">=2.19.3"
		manifest.Requires.RecommendedSub2APIVersion = "2.19.3"
		manifest.Requires.TestedSub2APIVersions = []string{"2.19.3"}
		manifest.Capabilities = []PluginCapability{{ID: PluginCapabilityCodexModelCatalog, Platform: PlatformOpenAI, AccountType: "catalog"}}
		manifest.Runtimes = map[string]PluginRuntime{manifest.RuntimeKey(): {Path: runtimePath}}
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		const publisher = "catalog-upgrade-test"
		cfg.Plugins.TrustedPublishers[publisher] = base64.StdEncoding.EncodeToString(publicKey)
		rawManifest, err := json.Marshal(manifest)
		require.NoError(t, err)
		rawSignature, err := json.Marshal(PluginSignature{Algorithm: "ed25519", KeyID: publisher, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, rawManifest))})
		require.NoError(t, err)
		var buf bytes.Buffer
		writer := zip.NewWriter(&buf)
		writeZipEntry(t, writer, pluginManifestFilename, rawManifest)
		writeZipEntry(t, writer, pluginSignatureFilename, rawSignature)
		for name, data := range files {
			writeZipEntry(t, writer, name, data)
		}
		require.NoError(t, writer.Close())
		archive = buf.Bytes()
	} else {
		t.Skip("catalog package or rebuilt binary not supplied")
	}
	installer := NewPluginPackageInstaller(cfg, PluginHostInfo{Version: "2.21.5", BuildType: "source"})
	i, err := installer.Install(context.Background(), bytes.NewReader(archive), nil)
	require.NoError(t, err)
	require.Equal(t, PluginSignatureTrusted, i.SignatureStatus)
	require.True(t, i.Compatibility.Compatible)
	for _, name := range []string{"index.html", "app.js", "styles.css", "base-model.js", "catalog-models.js"} {
		require.FileExists(t, filepath.Join(i.InstallPath, "ui", name))
	}
	r, err := startPluginRuntime(context.Background(), i, 15*time.Second, filepath.Join(root, "runtime"), nil)
	require.NoError(t, err)
	defer r.kill()
	raw, err := json.Marshal(testCatalogConfig(t))
	require.NoError(t, err)
	actual, err := r.validateAndApplyNormalizedConfig(context.Background(), raw)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(actual))
	hideRule := []byte(`{"schema_version":1,"models":[{"group_ids":[2],"hidden":true,"descriptor":{"slug":"gpt-6-sol"}}]}`)
	hidden, err := r.validateAndApplyNormalizedConfig(context.Background(), hideRule)
	require.NoError(t, err)
	require.JSONEq(t, string(hideRule), string(hidden))
	_, err = r.validateAndApplyNormalizedConfig(context.Background(), raw)
	require.NoError(t, err)
	health, err := r.status(context.Background())
	require.NoError(t, err)
	require.Contains(t, health.StatusJson, `"model_count":1`)
	response, err := r.api.ValidateConfig(context.Background(), &pluginv1.ValidateConfigRequest{ConfigJson: []byte(`{"schema_version":1,"models":[{"group_ids":[],"descriptor":{}}]}`)})
	require.NoError(t, err)
	require.False(t, response.Valid)
	manager := &PluginManager{encryptor: pluginTokenEncryptor{}, runtimes: map[int64]*pluginRuntime{}}
	i.ID = 9
	i.ConfigEncrypted = "ENC:" + string(actual)
	manager.publishRuntimeLocked(i, r)
	out, changed, err := manager.overlayCodexCatalog([]byte(`{"models":[{"slug":"gpt-future-test","use_responses_lite":false}]}`), 2, "0.164.0")
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(out), `"future_tool":{"version":5}`)
	selection := []byte(`{"schema_version":1,"models":[],"library":[{"slug":"chat","kind":"chat","display_name":"Selected"},{"slug":"pixel","kind":"image"}],"presets":[{"id":"p","name":"P","chat_models":["chat"],"image_models":[],"default_chat_model":"chat"}],"group_catalogs":[{"group_id":2,"preset_id":"p"},{"group_id":3,"chat_models":[]}]}`)
	normalized, err := r.validateAndApplyNormalizedConfig(context.Background(), selection)
	require.NoError(t, err)
	require.JSONEq(t, string(selection), string(normalized))
	i.ConfigEncrypted = "ENC:" + string(normalized)
	manager.publishRuntimeLocked(i, r)
	native := []byte(`{"models":[{"slug":"pixel"},{"slug":"chat","native":true},{"slug":"unlisted"}]}`)
	out, _, err = manager.overlayCodexCatalog(native, 2, "0.164.0")
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"chat","native":true,"display_name":"Selected","priority":0}],"default_chat_model":"chat"}`, string(out))
	out, _, err = manager.overlayCodexCatalog(native, 3, "0.164.0")
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[]}`, string(out))
	manager.removeRuntimeLocked(i.ID)
	require.Nil(t, manager.catalog.Load())
}
