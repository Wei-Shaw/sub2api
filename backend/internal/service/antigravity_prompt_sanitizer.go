package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// Antigravity 的 Cloud Code Assist 上游会检查 systemInstruction 的字面指纹，
// 命中后返回伪装成 429 的拒绝响应。下面的可配置改写规则在请求转发前把被
// 标记的字面串替换成等价表达；规则可通过环境变量或 JSON 文件配置，
// 文件规则支持热更新（无需重启进程）。
const (
	antigravityPromptSanitizeEnabledEnv = "GATEWAY_ANTIGRAVITY_PROMPT_SANITIZE_ENABLED"
	antigravityPromptSanitizeRulesEnv   = "GATEWAY_ANTIGRAVITY_PROMPT_SANITIZE_RULES"
	antigravityPromptSanitizeFileEnv    = "GATEWAY_ANTIGRAVITY_PROMPT_SANITIZE_FILE"

	antigravityPromptSanitizeMaxRules       = 64
	antigravityPromptSanitizeMaxPatternSize = 4096
)

type antigravityPromptSanitizeRule struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
}

type antigravityPromptSanitizeDocument struct {
	Rules []antigravityPromptSanitizeRule `json:"rules"`
}

var antigravityPromptSanitizerCache struct {
	mu        sync.Mutex
	key       string
	sanitizer *antigravity.PromptSanitizer
}

// antigravityPromptSanitizer 返回当前配置的 systemInstruction 改写器。
//
// 配置优先级：ENABLED=false/0 时强制关闭；RULES 内联 JSON 优先；否则读取 FILE
// 指向的 JSON 文件。文件按 size+mtime 作为缓存键，改动后下一次请求即生效。
func antigravityPromptSanitizer() *antigravity.PromptSanitizer {
	key, payload, source := resolveAntigravityPromptSanitizeSource()
	cache := &antigravityPromptSanitizerCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if key == cache.key {
		return cache.sanitizer
	}
	sanitizer := parseAntigravityPromptSanitizer(payload, source)
	cache.key = key
	cache.sanitizer = sanitizer
	return sanitizer
}

func resolveAntigravityPromptSanitizeSource() (key, payload, source string) {
	enabledRaw := strings.TrimSpace(os.Getenv(antigravityPromptSanitizeEnabledEnv))
	if strings.EqualFold(enabledRaw, "false") || enabledRaw == "0" {
		return "disabled", "", ""
	}
	if inline := strings.TrimSpace(os.Getenv(antigravityPromptSanitizeRulesEnv)); inline != "" {
		return "inline:" + inline, inline, "env " + antigravityPromptSanitizeRulesEnv
	}
	path := strings.TrimSpace(os.Getenv(antigravityPromptSanitizeFileEnv))
	if path == "" {
		if enabledRaw != "" {
			return "enabled-but-empty", "", "no rules configured"
		}
		return "none", "", ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return "file:" + path + ":missing", "", "file " + path
	}
	// 先 stat 再读取；读取失败（例如文件被并发删除）时用错误作为缓存键，
	// 避免每个请求都重复读取与重复告警。
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("file:%s:readerr:%v", path, err), "", "file " + path
	}
	return fmt.Sprintf("file:%s:%d:%d", path, info.Size(), info.ModTime().UnixNano()), string(data), "file " + path
}

func parseAntigravityPromptSanitizer(payload, source string) *antigravity.PromptSanitizer {
	if strings.TrimSpace(payload) == "" {
		if source == "no rules configured" {
			slog.Warn("antigravity_prompt_sanitize_no_rules",
				"hint", "set "+antigravityPromptSanitizeRulesEnv+" or "+antigravityPromptSanitizeFileEnv)
		}
		return nil
	}

	specs, err := decodeAntigravityPromptSanitizeRules(payload)
	if err != nil {
		slog.Warn("antigravity_prompt_sanitize_invalid_json", "source", source, "error", err)
		return nil
	}
	if len(specs) > antigravityPromptSanitizeMaxRules {
		slog.Warn("antigravity_prompt_sanitize_too_many_rules",
			"source", source, "count", len(specs), "max", antigravityPromptSanitizeMaxRules)
		specs = specs[:antigravityPromptSanitizeMaxRules]
	}

	compiled := make([]antigravity.PromptRule, 0, len(specs))
	for i, spec := range specs {
		pattern := strings.TrimSpace(spec.Pattern)
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = fmt.Sprintf("rule-%d", i+1)
		}
		if pattern == "" {
			slog.Warn("antigravity_prompt_sanitize_rule_skipped",
				"source", source, "rule", name, "error", "empty pattern")
			continue
		}
		if len(pattern) > antigravityPromptSanitizeMaxPatternSize {
			slog.Warn("antigravity_prompt_sanitize_rule_skipped",
				"source", source, "rule", name, "error", "pattern too long", "length", len(pattern))
			continue
		}
		rule, err := antigravity.NewPromptRule(name, pattern, spec.Replacement)
		if err != nil {
			slog.Warn("antigravity_prompt_sanitize_rule_skipped",
				"source", source, "rule", name, "error", err.Error())
			continue
		}
		compiled = append(compiled, rule)
	}

	sanitizer := antigravity.NewPromptSanitizer(compiled)
	if sanitizer == nil {
		slog.Warn("antigravity_prompt_sanitize_disabled", "source", source, "error", "no valid rules")
		return nil
	}
	slog.Info("antigravity_prompt_sanitize_enabled", "source", source, "rules", sanitizer.RuleCount())
	return sanitizer
}

// decodeAntigravityPromptSanitizeRules 同时接受数组和 {"rules":[...]} 两种写法。
func decodeAntigravityPromptSanitizeRules(payload string) ([]antigravityPromptSanitizeRule, error) {
	var rules []antigravityPromptSanitizeRule
	if err := json.Unmarshal([]byte(payload), &rules); err == nil {
		return rules, nil
	}
	var document antigravityPromptSanitizeDocument
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		return nil, err
	}
	return document.Rules, nil
}
