package kiro

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func schemaFrom(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestSanitizeSchemaDropsAdditionalProperties(t *testing.T) {
	t.Parallel()

	in := schemaFrom(t, `{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"nested": {"type": "object", "additionalProperties": true, "properties": {}}
		}
	}`)

	out := SanitizeSchema(in)

	require.NotContains(t, out, "additionalProperties")
	props, ok := out["properties"].(map[string]any)
	require.True(t, ok)
	nested, ok := props["nested"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, nested, "additionalProperties")
}

func TestSanitizeSchemaDropsEmptyRequired(t *testing.T) {
	t.Parallel()

	in := schemaFrom(t, `{"type":"object","required":[],"properties":{"a":{"type":"string"}}}`)
	out := SanitizeSchema(in)
	require.NotContains(t, out, "required")

	// 非空 required 必须保留。
	in = schemaFrom(t, `{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}`)
	out = SanitizeSchema(in)
	require.Equal(t, []any{"a"}, out["required"])
}

// TestSanitizeSchemaFlattensRefs 覆盖 Claude Code 工具 schema 的典型形态：
// zod / pydantic 生成的 schema 普遍带 $ref + $defs。
func TestSanitizeSchemaFlattensRefs(t *testing.T) {
	t.Parallel()

	in := schemaFrom(t, `{
		"type": "object",
		"$defs": {
			"Point": {"type": "object", "properties": {"x": {"type": "number"}}}
		},
		"properties": {
			"origin": {"$ref": "#/$defs/Point"}
		}
	}`)

	out := SanitizeSchema(in)

	require.NotContains(t, out, "$defs")
	props, ok := out["properties"].(map[string]any)
	require.True(t, ok)
	origin, ok := props["origin"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, origin, "$ref")
	require.Equal(t, "object", origin["type"])
	require.Contains(t, origin["properties"], "x")
}

func TestSanitizeSchemaHandlesArrayItems(t *testing.T) {
	t.Parallel()

	in := schemaFrom(t, `{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {"type": "object", "additionalProperties": false, "required": []}
			}
		}
	}`)

	out := SanitizeSchema(in)
	props, ok := out["properties"].(map[string]any)
	require.True(t, ok)
	arrayItems, ok := props["items"].(map[string]any)
	require.True(t, ok)
	items, ok := arrayItems["items"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, items, "additionalProperties")
	require.NotContains(t, items, "required")
}

func TestSanitizeSchemaNilAndEmpty(t *testing.T) {
	t.Parallel()

	require.NotNil(t, SanitizeSchema(nil))
	require.Empty(t, SanitizeSchema(map[string]any{}))
}

// TestSanitizeSchemaCyclicRefDoesNotHang 防御自引用 schema 导致的无限递归。
func TestSanitizeSchemaCyclicRefDoesNotHang(t *testing.T) {
	t.Parallel()

	in := schemaFrom(t, `{
		"type": "object",
		"$defs": {"Node": {"type":"object","properties":{"next":{"$ref":"#/$defs/Node"}}}},
		"properties": {"root": {"$ref": "#/$defs/Node"}}
	}`)

	done := make(chan map[string]any, 1)
	go func() { done <- SanitizeSchema(in) }()

	select {
	case out := <-done:
		require.NotNil(t, out)
	case <-timeAfterSeconds(5):
		t.Fatal("SanitizeSchema 在自引用 schema 上没有终止")
	}
}

// TestSanitizeSchemaArrayBudgetExhaustedStub 验证预算耗尽时数组的退化 stub 是
// 空数组而不是 {"type":"object"}（M-a）。在 enum/oneOf/anyOf 之下，数组的位置
// 期望的是数组值，用对象替换会产出无效 schema，可能触发本函数正要防止的 400。
func TestSanitizeSchemaArrayBudgetExhaustedStub(t *testing.T) {
	t.Parallel()

	ctx := &sanitizeCtx{defs: map[string]any{}, nodes: maxSchemaNodes}
	result := sanitizeValue([]any{"a", "b"}, ctx, 0)

	arr, ok := result.([]any)
	require.True(t, ok, "预算耗尽时数组退化的 stub 类型应为 []any，实际为 %T：%v", result, result)
	require.Empty(t, arr)
}

// TestSanitizeSchemaRequiredArrayNotAliased 验证 required 数组被复制而非别名。
// 修改返回值的 required 不应该污染输入。
func TestSanitizeSchemaRequiredArrayNotAliased(t *testing.T) {
	t.Parallel()

	in := schemaFrom(t, `{"type":"object","required":["a","b"],"properties":{"a":{"type":"string"}}}`)
	out := SanitizeSchema(in)

	// 获取输出的 required 数组并修改。
	outRequired, ok := out["required"].([]any)
	require.True(t, ok)
	outRequired[0] = "modified"

	// 验证输入的 required 没有被修改。
	inRequired, ok := in["required"].([]any)
	require.True(t, ok)
	require.Equal(t, "a", inRequired[0], "输入 required 被输出修改污染")
}

// buildLargeFlatPropertiesSchema 构造一个带 n 个平级属性的 object schema，
// required 列出全部 n 个属性名。每个属性自身只是一个廉价的标量 schema，
// 节点数几乎全部来自属性的数量本身——n 远大于 maxSchemaNodes 时，
// properties 内部循环必然会在预算耗尽后中途 break，产出一个只含部分
// 属性名的 properties；用于验证 required 是否被同步截断（整分支复核 Finding）。
func buildLargeFlatPropertiesSchema(n int) map[string]any {
	props := make(map[string]any, n)
	required := make([]any, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("p%d", i)
		props[name] = map[string]any{"type": "string"}
		required = append(required, name)
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// TestSanitizeSchemaRequiredNeverReferencesDroppedProperty 是整分支复核
// Finding 的回归测试：预算耗尽会让 properties 内部循环中途 break，只保留
// 已经产出的属性名；修复前 required 的复制发生在同一层循环里但完全绕过
// 预算检查（case "required" 里的 continue 从不看 ctx.exhausted()），于是
// required 原样带着全部属性名，其中一部分在 properties 里已经不存在——
// 产出的 schema 自相矛盾，与本预算机制本要防止的 Kiro400 同构风险。
//
// 用大量平级属性（而非嵌套 $ref fanout）构造，让"是否发生截断"不依赖 Go
// map 遍历顺序的随机性：只要 n 远大于 maxSchemaNodes，属性数量本身就足够
// 触发预算耗尽，被砍掉的具体是哪些属性名才是随机的。
func TestSanitizeSchemaRequiredNeverReferencesDroppedProperty(t *testing.T) {
	t.Parallel()

	const n = 20000
	in := buildLargeFlatPropertiesSchema(n)
	out := SanitizeSchema(in)

	props, ok := out["properties"].(map[string]any)
	require.True(t, ok, "顶层 properties 键本身不应该消失——顶层这次调用先于 properties 耗尽预算")
	require.Less(t, len(props), n,
		"本测试要求预算确实触发了截断（属性数 %d 远大于 maxSchemaNodes=%d），否则没测到目标场景", n, maxSchemaNodes)

	required, ok := out["required"].([]any)
	if !ok {
		require.Empty(t, props, "properties 非空但 required 完全消失，二者不一致")
		return
	}
	for _, name := range required {
		nameStr, isStr := name.(string)
		require.True(t, isStr)
		require.Contains(t, props, nameStr,
			"required 引用了输出 properties 里已不存在的属性 %q —— 预算截断产出了结构不一致的 schema", nameStr)
	}
}

// countSchemaNodes 统计 schema 中的容器节点数（maps 和 slices），标量不计数。
// 与 sanitizeCtx.nodes 统计的对象一致：每个 map/array 容器（含退化 stub）都算一个节点。
func countSchemaNodes(value any) int {
	switch v := value.(type) {
	case map[string]any:
		count := 1
		for _, val := range v {
			count += countSchemaNodes(val)
		}
		return count
	case []any:
		count := 1
		for _, item := range v {
			count += countSchemaNodes(item)
		}
		return count
	default:
		return 0
	}
}

// buildSelfRefFanoutSchema 构造一个自引用的 $defs 条目：Node 节点带 fanout 个属性，
// 每个属性都 $ref 回 Node 自身。这是 C1 描述的攻击形态——budget 修复前，输出节点数
// 会随 fanout 线性放大（budget 只约束了"展开访问次数"，每次访问仍无条件产出
// fanout 个子节点），fanout=10 时输出 ~55k 节点，fanout=50 时 ~255k 节点。
func buildSelfRefFanoutSchema(fanout int) map[string]any {
	props := make(map[string]any, fanout)
	for i := 0; i < fanout; i++ {
		props[fmt.Sprintf("p%d", i)] = map[string]any{"$ref": "#/$defs/Node"}
	}
	return map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"Node": map[string]any{
				"type":       "object",
				"properties": props,
			},
		},
		"properties": map[string]any{
			"root": map[string]any{"$ref": "#/$defs/Node"},
		},
	}
}

// TestSanitizeSchemaSelfRefFanoutBounded 防御自引用 $defs 导致的节点爆炸（C1）。
// 深度限制（maxRefDepth=16）会终止递归，但修复前每层终止前仍会无条件产出 fanout
// 个子节点——输出节点数随 fanout 线性放大：fanout=10 时 ~55k 节点，fanout=30 时
// ~155k，fanout=50 时 ~255k，budget=10000 完全不起作用。
//
// 修复后，budget 约束的是"实际产出的节点数"本身，与 fanout 无关：输出节点数应
// 稳定收敛在 maxSchemaNodes 的一个小的常数倍以内。
func TestSanitizeSchemaSelfRefFanoutBounded(t *testing.T) {
	t.Parallel()

	out := SanitizeSchema(buildSelfRefFanoutSchema(10))
	outNodes := countSchemaNodes(out)

	t.Logf("fanout=10 output node count: %d", outNodes)

	// 下界：如果输出坍缩到远小于 maxSchemaNodes（例如个位数），说明预算耗尽时
	// 把整个容器（连同已经构建好的兄弟节点）一起丢弃了，这是一种过度保守、
	// 会破坏合法 schema 结构的错误实现，而不是本次修复要的"停止继续产出新
	// 节点，但保留已产出部分"。
	require.Greater(t, outNodes, maxSchemaNodes/2,
		"输出节点数 %d 远小于预算，怀疑预算耗尽时把已构建好的兄弟节点一起丢弃了", outNodes)

	// 上界：修复前 fanout=10 会产出 ~55k 节点；修复后必须收敛到 maxSchemaNodes
	// 的一个小的常数倍以内，才算是"预算约束了输出节点数"而不是"访问次数"。
	require.Less(t, outNodes, maxSchemaNodes*2,
		"输出节点数 %d 超过预算的 2 倍，budget 没能约束住 fanout 放大", outNodes)
}

// TestSanitizeSchemaSelfRefFanoutBoundedIndependentOfFanout 证明输出上界与 fanout
// 无关——这是 C1 修复的核心不变量：只要 fanout 越大（10 到 10000，跨 3 个数量级），
// 输出节点数不应该继续增长，而应该始终收敛在 maxSchemaNodes 附近。
func TestSanitizeSchemaSelfRefFanoutBoundedIndependentOfFanout(t *testing.T) {
	t.Parallel()

	for _, fanout := range []int{10, 1000, 10000} {
		out := SanitizeSchema(buildSelfRefFanoutSchema(fanout))
		outNodes := countSchemaNodes(out)

		t.Logf("fanout=%d output node count: %d", fanout, outNodes)

		require.Less(t, outNodes, maxSchemaNodes*2,
			"fanout=%d 时输出节点数 %d 超过预算的 2 倍——修复前输出会随 fanout 线性放大，"+
				"fanout=10000 时会产生数千万个节点", fanout, outNodes)
	}
}

// TestSanitizeSchemaWideChainTerminatesQuickly 用真正会挂起的形态（宽分支链式
// $defs）验证 SanitizeSchema 在 budget 修复后仍能在有限时间内终止——把耗时保护
// 真正包住待测调用本身，而不是包一个已经跑完的结果（M-d：旧版本的 timeout
// scaffolding 是摆设，goroutine 在 SanitizeSchema 已经同步跑完之后才启动，
// 不可能捕捉到真实的挂起）。
func TestSanitizeSchemaWideChainTerminatesQuickly(t *testing.T) {
	t.Parallel()

	// buildWideChain 构造链式 $defs：L8 -> (c0:L7, ..., c_{branch-1}:L7),
	// L7 -> (c0:L6, ...), ..., L1 -> (无 refs，基础情况)
	buildWideChain := func(levels int, branch int) map[string]any {
		defs := map[string]any{}

		for level := levels; level > 0; level-- {
			node := map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}

			if level > 1 {
				nextLevel := level - 1
				props := make(map[string]any)
				for c := 0; c < branch; c++ {
					childName := fmt.Sprintf("L%d", nextLevel)
					props[fmt.Sprintf("c%d", c)] = map[string]any{
						"$ref": "#/$defs/" + childName,
					}
				}
				node["properties"] = props
			}

			levelName := fmt.Sprintf("L%d", level)
			defs[levelName] = node
		}

		return map[string]any{
			"type":  "object",
			"$defs": defs,
			"properties": map[string]any{
				"root": map[string]any{
					"$ref": fmt.Sprintf("#/$defs/L%d", levels),
				},
			},
		}
	}

	in := buildWideChain(8, 20)

	done := make(chan map[string]any, 1)
	go func() { done <- SanitizeSchema(in) }()

	select {
	case out := <-done:
		outNodes := countSchemaNodes(out)
		t.Logf("wide chain output node count: %d", outNodes)
		require.Less(t, outNodes, maxSchemaNodes*2,
			"输出节点数 %d 超过预算的 2 倍", outNodes)
	case <-timeAfterSeconds(5):
		t.Fatal("SanitizeSchema 在链式宽分支 $defs 上耗时过长，可能又退化为随 fanout 复合增长")
	}
}

func timeAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}
