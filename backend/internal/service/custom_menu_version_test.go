package service

import "testing"

func TestComputeCustomMenuVersion_Empty(t *testing.T) {
	v1 := ComputeCustomMenuVersion("")
	v2 := ComputeCustomMenuVersion("[]")
	v3 := ComputeCustomMenuVersion("   ")
	if v1 == "" || len(v1) != 12 {
		t.Fatalf("expected 12 hex chars, got %q", v1)
	}
	if v1 != v2 || v1 != v3 {
		t.Fatalf("empty variants should be equal, got %q %q %q", v1, v2, v3)
	}
}

func TestComputeCustomMenuVersion_Deterministic(t *testing.T) {
	items := `[{"id":"a","label":"A","sort_order":1,"action":"same_tab","url":"https://a"}]`
	v1 := ComputeCustomMenuVersion(items)
	v2 := ComputeCustomMenuVersion(items)
	if v1 != v2 {
		t.Fatalf("expected deterministic, got %q vs %q", v1, v2)
	}
}

func TestComputeCustomMenuVersion_ArrayOrderInvariant(t *testing.T) {
	// 相同 items（含相同 sort_order）不同数组顺序 → 相同 hash
	a := `[{"id":"a","sort_order":1,"label":"A"},{"id":"b","sort_order":2,"label":"B"}]`
	b := `[{"id":"b","sort_order":2,"label":"B"},{"id":"a","sort_order":1,"label":"A"}]`
	if ComputeCustomMenuVersion(a) != ComputeCustomMenuVersion(b) {
		t.Fatalf("array order should not affect hash")
	}
}

func TestComputeCustomMenuVersion_FieldOrderInvariant(t *testing.T) {
	// 单个 item 内 JSON key 顺序不同 → 相同 hash
	a := `[{"id":"a","label":"A","sort_order":1}]`
	b := `[{"sort_order":1,"label":"A","id":"a"}]`
	if ComputeCustomMenuVersion(a) != ComputeCustomMenuVersion(b) {
		t.Fatalf("field order should not affect hash")
	}
}

func TestComputeCustomMenuVersion_LabelChangeMatters(t *testing.T) {
	a := `[{"id":"a","label":"A","sort_order":1}]`
	b := `[{"id":"a","label":"A2","sort_order":1}]`
	if ComputeCustomMenuVersion(a) == ComputeCustomMenuVersion(b) {
		t.Fatalf("label change should mint new version")
	}
}

func TestComputeCustomMenuVersion_ShowRedDotToggleIgnored(t *testing.T) {
	// show_red_dot 每项开关不参与 hash：切换开关不应重置其他用户的 dismiss。
	a := `[{"id":"a","label":"A","sort_order":1,"show_red_dot":false}]`
	b := `[{"id":"a","label":"A","sort_order":1,"show_red_dot":true}]`
	if ComputeCustomMenuVersion(a) != ComputeCustomMenuVersion(b) {
		t.Fatalf("show_red_dot toggle should not affect hash")
	}
}

func TestComputeCustomMenuVersion_NonDisplayFieldIgnored(t *testing.T) {
	// updated_at / created_at 等非展示字段不应影响 hash
	a := `[{"id":"a","label":"A","sort_order":1,"updated_at":"2026-01-01"}]`
	b := `[{"id":"a","label":"A","sort_order":1,"updated_at":"2026-12-31"}]`
	if ComputeCustomMenuVersion(a) != ComputeCustomMenuVersion(b) {
		t.Fatalf("non-display field should not affect hash")
	}
}

func TestComputeCustomMenuVersion_SortOrderChangeMatters(t *testing.T) {
	a := `[{"id":"a","label":"A","sort_order":1},{"id":"b","label":"B","sort_order":2}]`
	b := `[{"id":"a","label":"A","sort_order":2},{"id":"b","label":"B","sort_order":1}]`
	if ComputeCustomMenuVersion(a) == ComputeCustomMenuVersion(b) {
		t.Fatalf("sort_order swap should mint new version")
	}
}

func TestComputeCustomMenuVersion_InvalidJSONFallsBackToEmpty(t *testing.T) {
	invalid := ComputeCustomMenuVersion("{not json")
	empty := ComputeCustomMenuVersion("[]")
	if invalid != empty {
		t.Fatalf("invalid JSON should behave as empty, got %q vs %q", invalid, empty)
	}
}
