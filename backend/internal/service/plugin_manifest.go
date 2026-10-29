package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
)

const (
	PluginCapabilityOpenAIOAuthOutbound = "openai.oauth.outbound_transport.v1"
	PluginStateDisabled                 = "disabled"
	PluginStateStarting                 = "starting"
	PluginStateEnabled                  = "enabled"
	PluginStateError                    = "error"
	PluginStateIncompatible             = "incompatible"
	PluginSignatureTrusted              = "trusted"
	PluginSignatureUnsigned             = "unsigned"
)

var pluginIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)+$`)

var ErrPluginStateChanged = errors.New("插件状态已在其他实例中变化，请刷新后重试")

// PluginManifest 是 .s2plugin 包中可在执行二进制前检查的声明。
type PluginManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description,omitempty"`
	Author        string                   `json:"author,omitempty"`
	Requires      PluginRequirements       `json:"requires"`
	Capabilities  []PluginCapability       `json:"capabilities"`
	Runtimes      map[string]PluginRuntime `json:"runtimes"`
	UI            PluginUIManifest         `json:"ui"`
	Files         map[string]string        `json:"files"`
}

type PluginRequirements struct {
	Sub2API                   string   `json:"sub2api"`
	RecommendedSub2APIVersion string   `json:"recommended_sub2api_version,omitempty"`
	TestedSub2APIVersions     []string `json:"tested_sub2api_versions,omitempty"`
	PluginProtocol            int      `json:"plugin_protocol"`
	TransportAPI              int      `json:"transport_api"`
	UIBridge                  int      `json:"ui_bridge"`
}

type PluginCapability struct {
	ID          string `json:"id"`
	Platform    string `json:"platform"`
	AccountType string `json:"account_type"`
}

type PluginRuntime struct {
	Path string `json:"path"`
}

type PluginUIManifest struct {
	Entrypoint string `json:"entrypoint"`
}

type PluginSignature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

// PluginCompatibility 是管理页面展示和启用门禁共同使用的兼容性结论。
type PluginCompatibility struct {
	Compatible         bool   `json:"compatible"`
	Tested             bool   `json:"tested"`
	Status             string `json:"status"`
	Message            string `json:"message"`
	CurrentSub2API     string `json:"current_sub2api_version"`
	RequiredSub2API    string `json:"required_sub2api_version"`
	RecommendedSub2API string `json:"recommended_sub2api_version"`
	PluginProtocol     int    `json:"plugin_protocol"`
	TransportAPI       int    `json:"transport_api"`
	UIBridge           int    `json:"ui_bridge"`
}

type PluginInstallation struct {
	ID              int64               `json:"id"`
	PluginKey       string              `json:"plugin_key"`
	Name            string              `json:"name"`
	Version         string              `json:"version"`
	Description     string              `json:"description"`
	Author          string              `json:"author"`
	Manifest        PluginManifest      `json:"manifest"`
	ArtifactData    []byte              `json:"-"`
	ArtifactPath    string              `json:"-"`
	InstallPath     string              `json:"-"`
	BinaryPath      string              `json:"-"`
	BinarySHA256    string              `json:"binary_sha256"`
	SignatureStatus string              `json:"signature_status"`
	State           string              `json:"state"`
	ConfigEncrypted string              `json:"-"`
	LastError       string              `json:"last_error"`
	InstalledBy     *int64              `json:"installed_by"`
	InstalledAt     time.Time           `json:"installed_at"`
	EnabledAt       *time.Time          `json:"enabled_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	Bindings        []PluginBinding     `json:"bindings"`
	Compatibility   PluginCompatibility `json:"compatibility"`
	RuntimeHealthy  bool                `json:"runtime_healthy"`
	RuntimeMessage  string              `json:"runtime_message"`
}

type PluginBinding struct {
	ID             int64     `json:"id"`
	PluginID       int64     `json:"plugin_id"`
	Capability     string    `json:"capability"`
	Platform       string    `json:"platform"`
	AccountType    string    `json:"account_type"`
	Enabled        bool      `json:"enabled"`
	RolloutPercent int       `json:"rollout_percent"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type PluginRepository interface {
	List(ctx context.Context) ([]*PluginInstallation, error)
	GetByID(ctx context.Context, id int64) (*PluginInstallation, error)
	GetByKey(ctx context.Context, key string) (*PluginInstallation, error)
	Install(ctx context.Context, plugin *PluginInstallation, bindings []PluginBinding) (*PluginInstallation, error)
	GetArtifact(ctx context.Context, id int64) ([]byte, error)
	Delete(ctx context.Context, id int64, expectedBinarySHA256 string) error
	BeginEnable(ctx context.Context, id int64, binarySHA256, expectedState string) error
	MarkRuntimeHealthy(ctx context.Context, id int64, binarySHA256, configEncrypted string) error
	UpdateState(ctx context.Context, id int64, state, lastError string, enabledAt *time.Time, expectedBinarySHA256, expectedState string) error
	UpdateConfig(ctx context.Context, id int64, encrypted, expectedBinarySHA256 string) error
	UpdateBindingsAndState(ctx context.Context, pluginID int64, bindings []PluginBinding, state, lastError string, enabledAt *time.Time, expectedState, expectedBinarySHA256 string) error
}

func (m PluginManifest) RuntimeKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func (m PluginManifest) Validate() error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("不支持的插件清单版本: %d", m.SchemaVersion)
	}
	if !pluginIDPattern.MatchString(m.ID) || len(m.ID) > 160 {
		return errors.New("插件 ID 必须是长度不超过 160 的小写命名空间标识")
	}
	if strings.TrimSpace(m.Name) == "" || len(m.Name) > 160 {
		return errors.New("插件名称不能为空且不能超过 160 个字符")
	}
	if normalizeSemver(m.Version) == "" {
		return errors.New("插件版本必须是有效的语义化版本")
	}
	if strings.TrimSpace(m.Requires.Sub2API) == "" {
		return errors.New("插件必须声明 requires.sub2api")
	}
	if m.Requires.PluginProtocol != pluginv1.ProtocolVersion ||
		m.Requires.TransportAPI != pluginv1.TransportAPIVersion ||
		m.Requires.UIBridge != pluginv1.UIBridgeVersion {
		return errors.New("插件协议、传输 API 或 UI Bridge 版本与当前宿主不兼容")
	}
	if len(m.Capabilities) == 0 {
		return errors.New("插件必须声明至少一个能力")
	}
	for _, capability := range m.Capabilities {
		if capability.ID != PluginCapabilityOpenAIOAuthOutbound || capability.Platform != PlatformOpenAI || capability.AccountType != AccountTypeOAuth {
			return fmt.Errorf("初期仅支持能力 %s", PluginCapabilityOpenAIOAuthOutbound)
		}
	}
	runtimeEntry, ok := m.Runtimes[m.RuntimeKey()]
	if !ok || !safePluginRelativePath(runtimeEntry.Path) {
		return fmt.Errorf("插件不支持当前运行平台 %s", m.RuntimeKey())
	}
	if !safePluginRelativePath(m.UI.Entrypoint) || !strings.HasPrefix(m.UI.Entrypoint, "ui/") {
		return errors.New("插件 UI 入口必须位于 ui/ 目录")
	}
	if len(m.Files) == 0 {
		return errors.New("插件清单必须声明文件哈希")
	}
	for path, hash := range m.Files {
		if !safePluginRelativePath(path) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(hash) {
			return fmt.Errorf("插件文件声明无效: %s", path)
		}
	}
	if _, ok := m.Files[runtimeEntry.Path]; !ok {
		return errors.New("运行时二进制未包含在文件哈希声明中")
	}
	if _, ok := m.Files[m.UI.Entrypoint]; !ok {
		return errors.New("UI 入口未包含在文件哈希声明中")
	}
	return nil
}

func safePluginRelativePath(path string) bool {
	cleaned := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	return cleaned != "" && cleaned != "." && !strings.HasPrefix(cleaned, "/") &&
		!strings.HasPrefix(cleaned, "../") && !strings.Contains(cleaned, "/../") && cleaned == strings.TrimPrefix(cleaned, "./")
}

func (m PluginManifest) MarshalJSONBytes() ([]byte, error) {
	return json.Marshal(m)
}

func (m PluginManifest) SortedCapabilities() []PluginCapability {
	out := append([]PluginCapability(nil), m.Capabilities...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
