package service

import (
	"fmt"
	"strings"
)

type opsAlertText struct {
	Name        string
	Description string
}

var defaultOpsAlertTextByLegacyName = map[string]opsAlertText{
	"错误率过高": {
		Name:        "Tỷ lệ lỗi cao",
		Description: "Cảnh báo khi tỷ lệ lỗi vượt 5% trong 5 phút.",
	},
	"成功率过低": {
		Name:        "Tỷ lệ thành công thấp",
		Description: "Cảnh báo khi tỷ lệ thành công dưới 95% trong 5 phút.",
	},
	"P99延迟过高": {
		Name:        "Độ trễ P99 cao",
		Description: "Cảnh báo khi độ trễ P99 vượt 3000ms trong 10 phút.",
	},
	"P95延迟过高": {
		Name:        "Độ trễ P95 cao",
		Description: "Cảnh báo khi độ trễ P95 vượt 2000ms trong 10 phút.",
	},
	"CPU使用率过高": {
		Name:        "CPU sử dụng cao",
		Description: "Cảnh báo khi CPU vượt 85% trong 10 phút.",
	},
	"内存使用率过高": {
		Name:        "Bộ nhớ sử dụng cao",
		Description: "Cảnh báo khi bộ nhớ vượt 90% trong 10 phút, có nguy cơ OOM.",
	},
	"并发队列积压": {
		Name:        "Hàng đợi đồng thời bị backlog",
		Description: "Cảnh báo khi hàng đợi đồng thời vượt 100 trong 5 phút.",
	},
	"错误率极高": {
		Name:        "Tỷ lệ lỗi cực cao",
		Description: "Cảnh báo nghiêm trọng khi tỷ lệ lỗi vượt 20% trong 1 phút.",
	},
}

func normalizeDefaultOpsAlertRule(rule *OpsAlertRule) {
	if rule == nil {
		return
	}
	legacyName := strings.TrimSpace(rule.Name)
	text, ok := defaultOpsAlertTextByLegacyName[legacyName]
	if !ok {
		return
	}
	rule.Name = text.Name
	if strings.TrimSpace(rule.Description) == "" || containsCJK(rule.Description) {
		rule.Description = text.Description
	}
}

func normalizeDefaultOpsAlertRules(rules []*OpsAlertRule) []*OpsAlertRule {
	for _, rule := range rules {
		normalizeDefaultOpsAlertRule(rule)
	}
	return rules
}

func normalizeDefaultOpsAlertEvent(event *OpsAlertEvent) {
	if event == nil {
		return
	}
	for legacy, text := range defaultOpsAlertTextByLegacyName {
		if strings.Contains(event.Title, legacy) {
			event.Title = strings.ReplaceAll(event.Title, legacy, text.Name)
		}
		if strings.Contains(event.Description, legacy) {
			event.Description = strings.ReplaceAll(event.Description, legacy, text.Name)
		}
	}
}

func normalizeDefaultOpsAlertEvents(events []*OpsAlertEvent) []*OpsAlertEvent {
	for _, event := range events {
		normalizeDefaultOpsAlertEvent(event)
	}
	return events
}

func buildOpsAlertTitle(rule *OpsAlertRule) string {
	if rule == nil {
		return ""
	}
	name := strings.TrimSpace(rule.Name)
	if text, ok := defaultOpsAlertTextByLegacyName[name]; ok {
		name = text.Name
	}
	severity := strings.TrimSpace(rule.Severity)
	if severity == "" {
		return name
	}
	if name == "" {
		return severity
	}
	return fmt.Sprintf("%s: %s", severity, name)
}

func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf') || (r >= '\uf900' && r <= '\ufaff') {
			return true
		}
	}
	return false
}
