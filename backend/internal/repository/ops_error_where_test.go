package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsErrorLogsWhere_UserScopedFilters(t *testing.T) {
	uid := int64(42)
	kid := int64(7)
	filter := &service.OpsErrorLogFilter{
		UserID:             &uid,
		APIKeyID:           &kid,
		Model:              "claude-sonnet-4-5",
		ExcludeCountTokens: true,
		ErrorPhasesAny:     []string{"auth"},
		ErrorTypesAny:      []string{"rate_limit_error"},
		View:               "all",
	}
	where, args := buildOpsErrorLogsWhere(filter)

	for _, want := range []string{
		"e.user_id = $",
		"e.api_key_id = $",
		"COALESCE(e.requested_model, e.model, '') = $",
		"COALESCE(e.is_count_tokens, false) = false",
		"e.error_phase = ANY($",
		"e.error_type = ANY($",
	} {
		if !strings.Contains(where, want) {
			t.Fatalf("where missing %q\nfull: %s", want, where)
		}
	}
	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %d", len(args))
	}
}

func TestBuildOpsErrorLogsWhere_ModelFuzzy(t *testing.T) {
	// 默认（ModelFuzzy=false）保持精确匹配
	exact := &service.OpsErrorLogFilter{Model: "claude"}
	whereExact, _ := buildOpsErrorLogsWhere(exact)
	if !strings.Contains(whereExact, "COALESCE(e.requested_model, e.model, '') = $") {
		t.Fatalf("default should be exact match, got: %s", whereExact)
	}

	// ModelFuzzy=true → ILIKE
	fuzzy := &service.OpsErrorLogFilter{Model: "claude", ModelFuzzy: true}
	whereFuzzy, args := buildOpsErrorLogsWhere(fuzzy)
	if !strings.Contains(whereFuzzy, "COALESCE(e.requested_model, e.model, '') ILIKE $") {
		t.Fatalf("ModelFuzzy should use ILIKE, got: %s", whereFuzzy)
	}
	if len(args) != 1 || args[0] != "%claude%" {
		t.Fatalf("expected arg \"%%claude%%\", got %v", args)
	}

	// 通配符转义：输入含 % 应被转义为字面量
	esc := &service.OpsErrorLogFilter{Model: "50%off", ModelFuzzy: true}
	_, escArgs := buildOpsErrorLogsWhere(esc)
	if len(escArgs) != 1 || escArgs[0] != `%50\%off%` {
		t.Fatalf("expected escaped arg, got %v", escArgs)
	}

	esc2 := &service.OpsErrorLogFilter{Model: "gpt_4o", ModelFuzzy: true}
	_, escArgs2 := buildOpsErrorLogsWhere(esc2)
	if len(escArgs2) != 1 || escArgs2[0] != `%gpt\_4o%` {
		t.Fatalf("underscore should be escaped, got %v", escArgs2)
	}
}

// TestBuildOpsErrorLogsWhere_CyberPolicyStatusExemption verifies that streaming
// cyber_policy hits (status_code=200) remain visible in admin + user error-request
// lists.  The repository filter must emit an OR exemption for error_type='cyber_policy'
// so that stream-path cyber rows (upstream delivers 200 with a failed SSE event) are
// not silently excluded by the COALESCE(status_code,0) >= 400 guard.
func TestBuildOpsErrorLogsWhere_CyberPolicyStatusExemption(t *testing.T) {
	// Default filter (no phase) must include the cyber_policy exemption.
	where, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{})
	if !strings.Contains(where, "e.error_type = 'cyber_policy'") {
		t.Fatalf("default filter must exempt cyber_policy from status >= 400 guard\nfull: %s", where)
	}
	if !strings.Contains(where, "COALESCE(e.status_code, 0) >= 400") {
		t.Fatalf("default filter must still include the status >= 400 guard for non-cyber rows\nfull: %s", where)
	}

	// phase=upstream skips the status guard entirely — exemption is irrelevant there.
	whereUpstream, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{Phase: "upstream"})
	if strings.Contains(whereUpstream, "status_code") {
		t.Fatalf("upstream phase filter must not add any status_code clause\nfull: %s", whereUpstream)
	}
}

func TestBuildOpsErrorLogsWhere_MatchDeletedKeyOwner(t *testing.T) {
	uid := int64(42)

	// 开关开启 → 归属放宽为 OR(user_id 或 deleted_key_owner_user_id),且共用同一占位符
	on := &service.OpsErrorLogFilter{UserID: &uid, MatchDeletedKeyOwner: true}
	whereOn, argsOn := buildOpsErrorLogsWhere(on)
	if !strings.Contains(whereOn, "(e.user_id = $1 OR e.deleted_key_owner_user_id = $1)") {
		t.Fatalf("MatchDeletedKeyOwner=true should widen to OR, got: %s", whereOn)
	}
	if len(argsOn) != 1 || argsOn[0] != uid {
		t.Fatalf("expected single reused arg %d, got %v", uid, argsOn)
	}

	// 开关关闭(默认)→ 仅精确 user_id,绝不出现 deleted_key_owner_user_id(admin 回归)
	off := &service.OpsErrorLogFilter{UserID: &uid}
	whereOff, _ := buildOpsErrorLogsWhere(off)
	if !strings.Contains(whereOff, "e.user_id = $1") {
		t.Fatalf("default should match user_id exactly, got: %s", whereOff)
	}
	if strings.Contains(whereOff, "deleted_key_owner_user_id") {
		t.Fatalf("default must NOT include deleted_key_owner_user_id, got: %s", whereOff)
	}
}
