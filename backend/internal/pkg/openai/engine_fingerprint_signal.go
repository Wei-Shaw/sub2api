package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// EngineFingerprintSignal 描述引擎指纹统一列表的一条信号。
// Required=true(勾选)= 该信号必须命中;多条 Required 之间 AND。
// Match 为同一信号的等价写法/变体,行内 OR(命中任一即算该条满足)。
type EngineFingerprintSignal struct {
	Type     string   `json:"type"`     // header_exact | header_prefix | body_path
	Match    []string `json:"match"`    // 行内 OR 变体
	Required bool     `json:"required"` // 勾选=true
}

const (
	FingerprintSignalHeaderExact  = "header_exact"
	FingerprintSignalHeaderPrefix = "header_prefix"
	FingerprintSignalBodyPath     = "body_path"
)

// DefaultEngineFingerprintSignals 默认种子:只勾 x-codex- 前缀,其余预填不勾。
// 依据:实测真 codex(含旧版)约 98.8% 必带 x-codex-window-id 头。
var DefaultEngineFingerprintSignals = []EngineFingerprintSignal{
	{Type: FingerprintSignalHeaderPrefix, Match: []string{"x-codex-"}, Required: true},
	{Type: FingerprintSignalHeaderExact, Match: []string{"session-id", "session_id"}, Required: false},
	{Type: FingerprintSignalHeaderExact, Match: []string{"thread-id", "thread_id"}, Required: false},
	{Type: FingerprintSignalBodyPath, Match: []string{"client_metadata.x-codex-window-id", "client_metadata.x-codex-installation-id"}, Required: false},
}

// EvaluateEngineFingerprint 应用「勾选 AND / 行内变体 OR」规则。
// 只有 Required=true 的条目参与;全部命中→true;任一缺失→false;无任何 Required→true。
func EvaluateEngineFingerprint(h http.Header, body []byte, signals []EngineFingerprintSignal) bool {
	for _, s := range signals {
		if !s.Required {
			continue
		}
		if !engineSignalMatches(h, body, s) {
			return false
		}
	}
	return true
}

func engineSignalMatches(h http.Header, body []byte, s EngineFingerprintSignal) bool {
	switch s.Type {
	case FingerprintSignalHeaderExact:
		for _, name := range s.Match {
			if n := strings.TrimSpace(name); n != "" && h != nil && strings.TrimSpace(h.Get(n)) != "" {
				return true
			}
		}
	case FingerprintSignalHeaderPrefix:
		if h == nil {
			return false
		}
		for k := range h {
			lk := strings.ToLower(k)
			for _, p := range s.Match {
				if np := strings.ToLower(strings.TrimSpace(p)); np != "" && strings.HasPrefix(lk, np) {
					return true
				}
			}
		}
	case FingerprintSignalBodyPath:
		if len(body) == 0 {
			return false
		}
		for _, path := range s.Match {
			if p := strings.TrimSpace(path); p != "" && gjson.GetBytes(body, p).Exists() {
				return true
			}
		}
	}
	return false
}

// ParseEngineFingerprintSignals 解析 JSON;空串→(nil,true);非法→(nil,false)。
func ParseEngineFingerprintSignals(raw string) ([]EngineFingerprintSignal, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, true
	}
	var sigs []EngineFingerprintSignal
	if json.Unmarshal([]byte(raw), &sigs) != nil {
		return nil, false
	}
	return sigs, true
}

// ValidateEngineFingerprintSignalsJSON 校验:空=合法;非空须为合法数组,
// 每条 type 合法且 match 至少一个非空项。供管理端写入校验复用。
func ValidateEngineFingerprintSignalsJSON(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var sigs []EngineFingerprintSignal
	if err := json.Unmarshal([]byte(trimmed), &sigs); err != nil {
		return fmt.Errorf("must be empty or a valid JSON array of {type, match[], required}")
	}
	for i, s := range sigs {
		switch s.Type {
		case FingerprintSignalHeaderExact, FingerprintSignalHeaderPrefix, FingerprintSignalBodyPath:
		default:
			return fmt.Errorf("entry %d: type must be one of header_exact/header_prefix/body_path", i)
		}
		hasMatch := false
		for _, m := range s.Match {
			if strings.TrimSpace(m) != "" {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			return fmt.Errorf("entry %d: match must contain at least one non-empty value", i)
		}
	}
	return nil
}

// DefaultEngineFingerprintSignalsJSON 默认种子的 JSON 字符串。
func DefaultEngineFingerprintSignalsJSON() string {
	b, _ := json.Marshal(DefaultEngineFingerprintSignals)
	return string(b)
}
