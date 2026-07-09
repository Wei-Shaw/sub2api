// Package pluginhost 是外部插件层宿主：zip 插件包的清单校验、磁盘存储与安装登记。
//
// 与内建层（pluginkit）的关系（详见 phase-4 PHASE_PLAN §2）：
//   - 插件 ID 与内建层共享点分命名空间，与内建清单冲突的包拒绝安装；
//   - 启停复用 plugin_states 状态机（无行 = disabled），安装登记另存
//     plugin_installations 表（本包 InstallationStore 端口，repository 层实现）；
//   - 子进程运行时（Supervisor/反代/能力面）经 ExternalRuntime 接缝衔接，
//     由 TASK-002 落地。
package pluginhost

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pluginkit"

	"golang.org/x/mod/semver"
)

const (
	// ManifestFileName 是插件包根目录下的清单文件名。
	ManifestFileName = "manifest.json"
	// ProtocolHTTP1 是当前唯一支持的数据面协议：插件进程在宿主指定的
	// unix socket 上起 HTTP/1.x 服务，宿主经 ReverseProxy 转发（phase-4 决策 1）。
	ProtocolHTTP1 = "http/1"
)

// 能力权限名（manifest.permissions 的合法取值，与能力面 v1 的三项一一对应）。
// 能力端点按声明放行：未声明的能力返回 403，未声明任何权限的插件只能跑
// 数据面 HTTP、完全够不到能力面（严格缺省）。
const (
	PermissionKV     = "kv"
	PermissionLog    = "log"
	PermissionConfig = "config"
)

// knownPermissions 是权限白名单：清单声明白名单之外的权限一律拒装
// （防拼写错误制造虚假授权，也为未来新增权限保留前向报错语义）。
var knownPermissions = map[string]bool{
	PermissionKV:     true,
	PermissionLog:    true,
	PermissionConfig: true,
}

// versionPattern 约束版本号可安全用作磁盘目录名：字母/数字开头，
// 后续允许 . _ -，长度 1-64（防路径穿越与隐藏目录）。
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// platformPattern 约束 backend.executables 的键为 goos-goarch 形式（如 linux-amd64）。
var platformPattern = regexp.MustCompile(`^[a-z0-9]+-[a-z0-9]+$`)

// Manifest 是外部插件包的清单（包根目录 manifest.json）。
type Manifest struct {
	ID             pluginkit.ID    `json:"id"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Description    string          `json:"description,omitempty"`
	MinCoreVersion string          `json:"min_core_version,omitempty"`
	Protocol       string          `json:"protocol"`
	Backend        *BackendSpec    `json:"backend,omitempty"`
	Frontend       *FrontendSpec   `json:"frontend,omitempty"`
	Permissions    []string        `json:"permissions,omitempty"`
	ConfigSchema   json.RawMessage `json:"config_schema,omitempty"`
}

// BackendSpec 描述插件的后端子进程：按 goos-goarch 平台键选择包内可执行文件。
type BackendSpec struct {
	Executables map[string]string `json:"executables"`
}

// FrontendSpec 描述插件的前端资产：entry 是包内 IIFE 入口（相对路径），
// locales 是可选的语言包文件映射（语言码 → 包内相对路径）。
type FrontendSpec struct {
	Entry   string            `json:"entry"`
	Locales map[string]string `json:"locales,omitempty"`
}

// CurrentPlatform 返回宿主平台键（goos-goarch），用于选择 backend 可执行文件。
func CurrentPlatform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// ParseManifest 解析清单 JSON（不做业务校验，校验见 Validate）。
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("pluginhost: parse %s: %w", ManifestFileName, err)
	}
	return &m, nil
}

// Validate 校验清单的完整性与安全性：
//   - ID 复用 pluginkit.ID 的命名规则（与内建清单的冲突检查在 Installer 中完成，
//     需要注入保留 ID 集合）；
//   - protocol 仅接受 "http/1"；
//   - backend 存在时 executables 必须含宿主当前平台（缺失时报错说明缺哪个）；
//   - 所有包内路径必须是安全相对路径（防穿越）；
//   - permissions 仅接受白名单内的能力名；
//   - min_core_version 存在时必须是合法语义化版本；
//   - config_schema 存在时必须是 JSON 对象。
func (m *Manifest) Validate() error {
	if err := m.ID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("pluginhost: manifest name is required")
	}
	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("pluginhost: invalid manifest version %q: must match %s", m.Version, versionPattern.String())
	}
	if m.MinCoreVersion != "" && !semver.IsValid(canonSemver(m.MinCoreVersion)) {
		return fmt.Errorf("pluginhost: invalid min_core_version %q: must be a semantic version (e.g. 0.2.0)", m.MinCoreVersion)
	}
	for _, perm := range m.Permissions {
		if !knownPermissions[perm] {
			return fmt.Errorf("pluginhost: unknown permission %q: supported permissions are %s, %s, %s",
				perm, PermissionKV, PermissionLog, PermissionConfig)
		}
	}
	if m.Protocol != ProtocolHTTP1 {
		return fmt.Errorf("pluginhost: unsupported protocol %q: only %q is supported", m.Protocol, ProtocolHTTP1)
	}
	if m.Backend == nil && m.Frontend == nil {
		return fmt.Errorf("pluginhost: manifest must declare at least one of backend/frontend")
	}
	if m.Backend != nil {
		if err := m.Backend.validate(); err != nil {
			return err
		}
	}
	if m.Frontend != nil {
		if err := m.Frontend.validate(); err != nil {
			return err
		}
	}
	if len(m.ConfigSchema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(m.ConfigSchema, &schema); err != nil {
			return fmt.Errorf("pluginhost: config_schema must be a JSON object: %w", err)
		}
	}
	return nil
}

// ExecutableFor 返回指定平台的可执行文件相对路径；未声明时返回 false。
func (m *Manifest) ExecutableFor(platform string) (string, bool) {
	if m.Backend == nil {
		return "", false
	}
	p, ok := m.Backend.Executables[platform]
	return p, ok
}

// PermissionSet 返回清单声明的能力权限集合（能力面按此放行）。
func (m *Manifest) PermissionSet() map[string]bool {
	perms := make(map[string]bool, len(m.Permissions))
	for _, p := range m.Permissions {
		perms[p] = true
	}
	return perms
}

// MinCoreVersionSatisfied 报告宿主版本是否满足清单的 min_core_version 要求。
// 清单未声明视为满足；宿主版本非法或为开发构建（0.0.0-dev）时跳过检查并
// 返回 skipped=true（caller 记 warn），开发环境不阻塞插件调试。
func (m *Manifest) MinCoreVersionSatisfied(hostVersion string) (ok, skipped bool) {
	required := canonSemver(m.MinCoreVersion)
	if required == "" {
		return true, false
	}
	host := canonSemver(hostVersion)
	if !semver.IsValid(host) || host == "v0.0.0-dev" {
		return true, true
	}
	return semver.Compare(host, required) >= 0, false
}

// canonSemver 把版本号规范为 x/mod/semver 要求的 v 前缀形态；空串原样返回。
func canonSemver(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "v") {
		return s
	}
	return "v" + s
}

func (b *BackendSpec) validate() error {
	if len(b.Executables) == 0 {
		return fmt.Errorf("pluginhost: backend.executables must not be empty")
	}
	for platform, rel := range b.Executables {
		if !platformPattern.MatchString(platform) {
			return fmt.Errorf("pluginhost: invalid executable platform key %q: must be goos-goarch (e.g. linux-amd64)", platform)
		}
		if err := validateRelPath(rel); err != nil {
			return fmt.Errorf("pluginhost: backend.executables[%s]: %w", platform, err)
		}
	}
	if _, ok := b.Executables[CurrentPlatform()]; !ok {
		return fmt.Errorf("pluginhost: backend.executables missing entry for current platform %q", CurrentPlatform())
	}
	return nil
}

func (f *FrontendSpec) validate() error {
	if err := validateRelPath(f.Entry); err != nil {
		return fmt.Errorf("pluginhost: frontend.entry: %w", err)
	}
	for lang, rel := range f.Locales {
		if err := validateRelPath(rel); err != nil {
			return fmt.Errorf("pluginhost: frontend.locales[%s]: %w", lang, err)
		}
	}
	return nil
}

// validateRelPath 校验包内相对路径：非空、斜杠分隔、无绝对路径/../反斜杠/冗余段。
func validateRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("path is required")
	}
	if strings.Contains(p, `\`) {
		return fmt.Errorf("path %q must use forward slashes", p)
	}
	if path.IsAbs(p) {
		return fmt.Errorf("path %q must be relative", p)
	}
	cleaned := path.Clean(p)
	if cleaned != p || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path %q must be a clean relative path inside the package", p)
	}
	return nil
}

// ============================================================
// config_schema 基础校验（phase-4 决策 7 的 MVP）
// ============================================================

// basicSchema 是 JSON Schema 的极小子集：type / properties / required / items。
// 标准库无法覆盖的特性（enum/pattern/format/allOf 等）不校验，静默放行——
// 登记：待后续任务引入完整 JSON Schema 校验器时收口。
type basicSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]*basicSchema `json:"properties"`
	Required   []string                `json:"required"`
	Items      *basicSchema            `json:"items"`
}

// ValidateConfig 用 manifest 的 config_schema 对配置做基础校验；
// 无 schema 时仅要求 config 是合法 JSON。
func (m *Manifest) ValidateConfig(config json.RawMessage) error {
	var value any
	if err := json.Unmarshal(config, &value); err != nil {
		return fmt.Errorf("pluginhost: config must be valid JSON: %w", err)
	}
	if len(m.ConfigSchema) == 0 {
		return nil
	}
	var schema basicSchema
	if err := json.Unmarshal(m.ConfigSchema, &schema); err != nil {
		// Validate 已保证 schema 是 JSON 对象；子集字段解析失败视为不可校验，放行。
		return nil
	}
	return validateAgainstSchema(value, &schema, "$")
}

// validateAgainstSchema 递归校验 type/properties/required/items 子集。
func validateAgainstSchema(value any, schema *basicSchema, at string) error {
	if schema == nil {
		return nil
	}
	if schema.Type != "" {
		if err := validateJSONType(value, schema.Type, at); err != nil {
			return err
		}
	}
	obj, isObj := value.(map[string]any)
	if isObj {
		for _, key := range schema.Required {
			if _, ok := obj[key]; !ok {
				return fmt.Errorf("pluginhost: config %s: missing required property %q", at, key)
			}
		}
		for key, sub := range schema.Properties {
			if v, ok := obj[key]; ok {
				if err := validateAgainstSchema(v, sub, at+"."+key); err != nil {
					return err
				}
			}
		}
	}
	if arr, ok := value.([]any); ok && schema.Items != nil {
		for i, v := range arr {
			if err := validateAgainstSchema(v, schema.Items, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateJSONType 校验 JSON 值与 schema type 声明匹配。
func validateJSONType(value any, typ, at string) error {
	ok := false
	switch typ {
	case "object":
		_, ok = value.(map[string]any)
	case "array":
		_, ok = value.([]any)
	case "string":
		_, ok = value.(string)
	case "boolean":
		_, ok = value.(bool)
	case "number":
		_, ok = value.(float64)
	case "integer":
		if f, isNum := value.(float64); isNum {
			ok = f == math.Trunc(f)
		}
	case "null":
		ok = value == nil
	default:
		// 未知 type 声明：子集之外，不校验。
		return nil
	}
	if !ok {
		return fmt.Errorf("pluginhost: config %s: expected type %q", at, typ)
	}
	return nil
}
