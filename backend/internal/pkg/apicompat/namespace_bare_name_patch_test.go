package apicompat

import "testing"

func TestNamespaceToolByBareName(t *testing.T) {
	tools := map[string]NamespacedToolName{
		"mcp__sequential_thinking__sequentialthinking": {Namespace: "mcp__sequential_thinking", Name: "sequentialthinking"},
		"mcp__codegraph__codegraph_explore":             {Namespace: "mcp__codegraph", Name: "codegraph_explore"},
		"mcp__gopls__go_diagnostics":                    {Namespace: "mcp__gopls", Name: "go_diagnostics"},
	}
	// 唯一裸名 → 还原
	ns, ok := NamespaceToolByBareName(tools, "sequentialthinking")
	if !ok || ns.Namespace != "mcp__sequential_thinking" || ns.Name != "sequentialthinking" {
		t.Fatalf("unique bare name: got ok=%v ns=%+v", ok, ns)
	}
	// 摊平名也算裸名集合外 → 不匹配（摊平名走原 map 分支）
	if _, ok := NamespaceToolByBareName(tools, "mcp__codegraph__codegraph_explore"); ok {
		t.Fatal("flattened name should not match bare-name fallback")
	}
	// 无匹配
	if _, ok := NamespaceToolByBareName(tools, "nonexistent"); ok {
		t.Fatal("unknown name should not match")
	}
	// 空 map
	if _, ok := NamespaceToolByBareName(nil, "sequentialthinking"); ok {
		t.Fatal("nil map should not match")
	}
	// 撞名：两个 namespace 下同名子工具 → 不还原
	dup := map[string]NamespacedToolName{
		"ns_a__read": {Namespace: "ns_a", Name: "read"},
		"ns_b__read": {Namespace: "ns_b", Name: "read"},
	}
	if _, ok := NamespaceToolByBareName(dup, "read"); ok {
		t.Fatal("duplicate bare name should not match")
	}
}
