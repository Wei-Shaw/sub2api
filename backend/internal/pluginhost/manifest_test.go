//go:build unit

package pluginhost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// validTestManifest 构造一份通过 Validate 的最小清单（backend 含当前平台）。
func validTestManifest() *Manifest {
	return &Manifest{
		ID:       "acme.hello",
		Name:     "Hello Plugin",
		Version:  "1.0.0",
		Protocol: ProtocolHTTP1,
		Backend: &BackendSpec{
			Executables: map[string]string{CurrentPlatform(): "bin/plugin"},
		},
	}
}

func TestManifestParseAndValidate(t *testing.T) {
	raw := []byte(`{
		"id": "acme.hello",
		"name": "Hello Plugin",
		"version": "1.2.3",
		"protocol": "http/1",
		"backend": {"executables": {"` + CurrentPlatform() + `": "bin/plugin"}},
		"frontend": {"entry": "web/plugin.js", "locales": {"zh": "web/locales/zh.json"}},
		"permissions": ["kv"],
		"config_schema": {"type": "object"}
	}`)
	m, err := ParseManifest(raw)
	require.NoError(t, err)
	require.NoError(t, m.Validate())
	require.Equal(t, "1.2.3", m.Version)

	exe, ok := m.ExecutableFor(CurrentPlatform())
	require.True(t, ok)
	require.Equal(t, "bin/plugin", exe)
	_, ok = m.ExecutableFor("plan9-mips")
	require.False(t, ok)
}

func TestManifestParseRejectsMalformedJSON(t *testing.T) {
	_, err := ParseManifest([]byte(`{"id":`))
	require.Error(t, err)
}

func TestManifestValidateRejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(m *Manifest)
		wantSub string
	}{
		{"非法 ID", func(m *Manifest) { m.ID = "Bad_ID" }, "invalid plugin id"},
		{"缺 name", func(m *Manifest) { m.Name = " " }, "name is required"},
		{"非法版本号", func(m *Manifest) { m.Version = "../evil" }, "invalid manifest version"},
		{"不支持的协议", func(m *Manifest) { m.Protocol = "grpc" }, "unsupported protocol"},
		{"backend/frontend 全缺", func(m *Manifest) { m.Backend = nil; m.Frontend = nil }, "at least one of backend/frontend"},
		{"executables 为空", func(m *Manifest) { m.Backend.Executables = nil }, "must not be empty"},
		{"平台键非法", func(m *Manifest) {
			m.Backend.Executables = map[string]string{"LinuxAmd64": "bin/plugin"}
		}, "invalid executable platform key"},
		{"可执行文件路径穿越", func(m *Manifest) {
			m.Backend.Executables = map[string]string{CurrentPlatform(): "../bin/plugin"}
		}, "clean relative path"},
		{"frontend entry 缺失", func(m *Manifest) {
			m.Frontend = &FrontendSpec{}
		}, "frontend.entry"},
		{"frontend entry 绝对路径", func(m *Manifest) {
			m.Frontend = &FrontendSpec{Entry: "/web/plugin.js"}
		}, "must be relative"},
		{"locales 路径反斜杠", func(m *Manifest) {
			m.Frontend = &FrontendSpec{Entry: "web/plugin.js", Locales: map[string]string{"zh": `web\zh.json`}}
		}, "forward slashes"},
		{"config_schema 非对象", func(m *Manifest) { m.ConfigSchema = json.RawMessage(`[1,2]`) }, "must be a JSON object"},
		{"白名单外权限", func(m *Manifest) { m.Permissions = []string{"kv", "db"} }, `unknown permission "db"`},
		{"min_core_version 非 semver", func(m *Manifest) { m.MinCoreVersion = "latest" }, "invalid min_core_version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validTestManifest()
			tc.mutate(m)
			err := m.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

// TestManifestValidateMissingCurrentPlatform 缺当前平台的二进制时，
// 错误信息必须说明缺哪个平台（400 响应据此透出）。
func TestManifestValidateMissingCurrentPlatform(t *testing.T) {
	m := validTestManifest()
	m.Backend.Executables = map[string]string{"plan9-mips": "bin/plugin"}
	err := m.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), CurrentPlatform())
}

// TestManifestMinCoreVersionSatisfied 版本门槛判定：宿主达标/不达标、
// v 前缀混用、开发构建与非法宿主版本跳过（skipped=true）。
func TestManifestMinCoreVersionSatisfied(t *testing.T) {
	cases := []struct {
		name        string
		min, host   string
		ok, skipped bool
	}{
		{"未声明恒满足", "", "0.0.1", true, false},
		{"宿主等于门槛", "1.2.0", "1.2.0", true, false},
		{"宿主高于门槛", "1.2.0", "1.10.0", true, false},
		{"宿主低于门槛", "1.2.0", "1.1.9", false, false},
		{"v 前缀混用", "v0.5.0", "0.6.0", true, false},
		{"开发构建跳过", "9.9.9", "0.0.0-dev", true, true},
		{"宿主版本非法跳过", "9.9.9", "not-a-version", true, true},
		{"宿主版本为空跳过", "9.9.9", "", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validTestManifest()
			m.MinCoreVersion = tc.min
			ok, skipped := m.MinCoreVersionSatisfied(tc.host)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.skipped, skipped)
		})
	}
}

func TestManifestPermissionSet(t *testing.T) {
	m := validTestManifest()
	m.Permissions = []string{PermissionKV, PermissionLog}
	perms := m.PermissionSet()
	require.True(t, perms[PermissionKV])
	require.True(t, perms[PermissionLog])
	require.False(t, perms[PermissionConfig])
	require.Empty(t, validTestManifest().PermissionSet(), "未声明任何权限 = 空集")
}

func TestManifestValidateConfigWithoutSchema(t *testing.T) {
	m := validTestManifest()
	require.NoError(t, m.ValidateConfig(json.RawMessage(`{"anything": true}`)))
	require.Error(t, m.ValidateConfig(json.RawMessage(`{oops`)))
}

func TestManifestValidateConfigAgainstSchema(t *testing.T) {
	m := validTestManifest()
	m.ConfigSchema = json.RawMessage(`{
		"type": "object",
		"required": ["port"],
		"properties": {
			"port": {"type": "integer"},
			"host": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "string"}},
			"nested": {"type": "object", "required": ["key"]}
		}
	}`)

	require.NoError(t, m.ValidateConfig(json.RawMessage(`{"port": 8080, "host": "a", "tags": ["x"]}`)))

	// 顶层类型不符
	require.ErrorContains(t, m.ValidateConfig(json.RawMessage(`[1]`)), `expected type "object"`)
	// 缺必填
	require.ErrorContains(t, m.ValidateConfig(json.RawMessage(`{"host": "a"}`)), `missing required property "port"`)
	// integer 不接受小数
	require.ErrorContains(t, m.ValidateConfig(json.RawMessage(`{"port": 1.5}`)), `expected type "integer"`)
	// 属性类型不符
	require.ErrorContains(t, m.ValidateConfig(json.RawMessage(`{"port": 1, "host": 2}`)), `expected type "string"`)
	// 数组元素类型不符
	require.ErrorContains(t, m.ValidateConfig(json.RawMessage(`{"port": 1, "tags": [3]}`)), `expected type "string"`)
	// 嵌套 required
	require.ErrorContains(t, m.ValidateConfig(json.RawMessage(`{"port": 1, "nested": {}}`)), `missing required property "key"`)

	// 子集之外的 type 声明不校验（enum/format 等同理，静默放行）
	m.ConfigSchema = json.RawMessage(`{"type": "unknown-keyword"}`)
	require.NoError(t, m.ValidateConfig(json.RawMessage(`{"x": 1}`)))
}
