package service

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// 真实 Claude Code CLI 2.1.31x 的 safeguards 块：dangerous_tool_use 分类器上下文。
// 抓包结构见 docs/claude-code-fingerprint.md。字段缺失会让上游看到的"客户端画像"
// 与 attribution header 声称的版本自相矛盾。
//
// 上下文里带的是宿主机路径，与真实 CLI 一样按运行环境取值，不写死。
const safeguardsClassifierVersion = 1

type safeguardsBlock struct {
	Type              string            `json:"type"`
	ClassifierContext classifierContext `json:"classifier_context"`
}

type classifierContext struct {
	V                int                `json:"v"`
	PermissionMode   string             `json:"permission_mode"`
	Platform         string             `json:"platform"`
	LiveCwd          string             `json:"live_cwd"`
	HomeDir          string             `json:"home_dir"`
	RuleRoots        map[string]string  `json:"rule_roots"`
	TrustedDirs      trustedDirectories `json:"trusted_directories"`
	Rules            ruleSets           `json:"rules"`
	AutoMode         autoModeSets       `json:"auto_mode"`
	ConsentHoldback  bool               `json:"artifact_consent_holdback"`
	CaseInsensitive  bool               `json:"case_insensitive_paths"`
	Restricted       bool               `json:"restricted"`
	IsRemoteMode     bool               `json:"is_remote_mode"`
	ClassifyAllShell bool               `json:"classify_all_shell"`
	UserIdentity     string             `json:"user_identity"`
	GitState         gitState           `json:"git_state"`
}

type trustedDirectories struct {
	Primary    trustedPath   `json:"primary"`
	Additional []trustedPath `json:"additional"`
	Network    []string      `json:"network"`
	BlockReads bool          `json:"block_reads_outside_working_directories"`
}

type trustedPath struct {
	Path     string   `json:"path"`
	Resolved []string `json:"resolved"`
	Source   string   `json:"source,omitempty"`
}

type ruleSets struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
	Ask   []string `json:"ask"`
}

type autoModeSets struct {
	Allow       []string `json:"allow"`
	SoftDeny    []string `json:"soft_deny"`
	HardDeny    []string `json:"hard_deny"`
	Environment []string `json:"environment"`
}

type gitState struct {
	Cwd           string        `json:"cwd"`
	Root          *string       `json:"root"`
	Branch        *string       `json:"branch"`
	DefaultBranch *string       `json:"default_branch"`
	Status        *string       `json:"status"`
	Visibility    gitVisibility `json:"visibility"`
	Error         string        `json:"error"`
}

type gitVisibility struct {
	Origin          *string  `json:"origin"`
	PushRemote      *string  `json:"push_remote"`
	Remotes         []string `json:"remotes"`
	VisibilityCache []string `json:"visibility_cache"`
}

func emptyStrings() []string { return []string{} }

// claudePlatform 与 Node 的 process.platform 取值一致。
// clientOS 是客户端 X-Stainless-OS 头（真机 CLI 一定带），优先按它判定——
// 否则 Linux 网关会把 Windows 客户端说成 linux，与出站 X-Stainless-OS 自相矛盾。
func claudePlatform(clientOS string) string {
	switch strings.ToLower(strings.TrimSpace(clientOS)) {
	case "windows":
		return "windows"
	case "macos", "mac os", "darwin":
		return "darwin"
	case "linux":
		return "linux"
	}
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "darwin"
	default:
		return "linux"
	}
}

// buildSafeguardsBlock 按真实 CLI 形状构造 safeguards 数组。
func buildSafeguardsBlock(clientOS string) ([]byte, error) {
	cwd, _ := os.Getwd()
	cwd = filepath.ToSlash(cwd)
	home, _ := os.UserHomeDir()
	home = filepath.ToSlash(home)

	// userSettings 指向配置目录，其余指向 cwd —— 与真机一致。
	configDir, _ := os.UserConfigDir()
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		configDir = v
	} else if home != "" {
		configDir = filepath.ToSlash(filepath.Join(home, ".claude"))
	}
	roots := map[string]string{"userSettings": filepath.ToSlash(configDir)}
	for _, k := range []string{
		"projectSettings", "localSettings", "flagSettings", "policySettings",
		"cliArg", "command", "session", "toolsNarrowing", "mcpServerPolicy", "hostCredential",
	} {
		roots[k] = cwd
	}

	// 容器里通常没有 USER/USERNAME 环境变量，回落到 passwd 用户名，避免空串。
	identity := os.Getenv("USERNAME")
	if identity == "" {
		identity = os.Getenv("USER")
	}
	if identity == "" {
		if u, err := user.Current(); err == nil {
			identity = u.Username
		}
	}

	block := []safeguardsBlock{{
		Type: "dangerous_tool_use",
		ClassifierContext: classifierContext{
			V:              safeguardsClassifierVersion,
			PermissionMode: "auto",
			Platform:       claudePlatform(clientOS),
			LiveCwd:        cwd,
			HomeDir:        home,
			RuleRoots:      roots,
			TrustedDirs: trustedDirectories{
				Primary: trustedPath{Path: cwd, Resolved: []string{cwd}},
				// 真机会把会话目录作为 additional 项重复一次（source=session）。
				Additional: []trustedPath{{Path: cwd, Resolved: []string{cwd, cwd}, Source: "session"}},
				Network:    emptyStrings(),
				BlockReads: false,
			},
			Rules:            ruleSets{Allow: emptyStrings(), Deny: emptyStrings(), Ask: emptyStrings()},
			AutoMode:         autoModeSets{Allow: emptyStrings(), SoftDeny: emptyStrings(), HardDeny: emptyStrings(), Environment: emptyStrings()},
			ConsentHoldback:  false,
			CaseInsensitive:  claudePlatform(clientOS) != "linux",
			Restricted:       false,
			IsRemoteMode:     false,
			ClassifyAllShell: false,
			UserIdentity:     strings.TrimSpace(identity),
			GitState: gitState{
				Cwd:        cwd,
				Visibility: gitVisibility{Remotes: emptyStrings(), VisibilityCache: emptyStrings()},
				Error:      "not_a_repo",
			},
		},
	}}

	return json.Marshal(block)
}
