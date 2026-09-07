package kiro

import "strings"

// maxRefDepth 限制递归下降的嵌套深度——不是单纯的 $ref 跳数。
// depth 在每一层递归（进入 properties/items 这类普通嵌套、进入 $ref 包装节点、
// 展开 $ref 后进入目标 schema）都会 +1，实测展开一次 $ref 大约消耗 3 层深度，
// 因此合法的深层 schema 在约 5 层 $ref 展开之后就会失去继续展开的能力。
// 用途是防止自引用 schema 导致无限递归，代价是限制了合法嵌套的深度上限。
const maxRefDepth = 16

// maxSchemaNodes 限制清洗后 schema 输出的总节点数（map 与 array 容器，含退化 stub）。
//
// 工具 schema 来自客户端，可构造出自引用的 $defs 条目：一个节点带 N 个属性，
// 每个属性都 $ref 回节点自身。深度上限（maxRefDepth）会终止递归，但终止前
// 每层展开都会无条件产出 N 个子节点，输出节点数随 N 线性放大，且随层数复合。
// 因此预算必须直接约束"实际产出的节点数"本身，且必须在每个节点即将被产出的
// 那一刻检查——包括 $ref 包装节点和 fanout 里的每一个子节点，一旦超支就地
// 退化为 stub 并放弃继续递归，不能让预算只约束"展开访问次数"（历史事故：
// 预算检查曾经放在 $ref 分支判断之后，$ref 分支提前 return 绕过了检查，
// 预算只限制了访问次数，每次访问仍会无条件产出 fanout 个子节点）。
const maxSchemaNodes = 10000

// sanitizeCtx 跟踪 $ref 可解引用表与已花费的输出节点预算。
type sanitizeCtx struct {
	defs  map[string]any
	nodes int
}

// spend 为即将产出的一个容器节点（map 或 array，含退化 stub）花费一个预算单位。
// 预算耗尽时返回 false，调用方必须立即退化为 stub 且不再递归产出任何子节点——
// 这是让预算约束"最终输出节点数"而不是"展开访问次数"的关键：任何一个即将
// 被写入输出的容器，不论是不是 $ref 包装节点，都必须先经过这里。
func (ctx *sanitizeCtx) spend() bool {
	if ctx.nodes >= maxSchemaNodes {
		return false
	}
	ctx.nodes++
	return true
}

// exhausted 只读检查预算是否已耗尽，不消耗预算。
// 用在容器已经开始构建、正准备产出下一个子节点之前：一旦预算耗尽，必须停止
// 迭代——直接跳出循环，不再为剩余的 key/元素产出任何东西，哪怕是 stub 也不
// 产出。已经构建好的部分（更早的 key/元素）原样保留并返回，不整体丢弃。
// 停止迭代（而不是继续给每个剩余元素都产出一个 stub）是让预算约束"输出
// 节点数"而不是"访问次数"的关键：只要不再产出新的容器，fanout 有多宽都
// 不影响输出大小。
func (ctx *sanitizeCtx) exhausted() bool {
	return ctx.nodes >= maxSchemaNodes
}

// SanitizeSchema 把 Anthropic 工具的 input_schema 清洗成 Kiro 可接受的形态。
//
// Kiro 对工具 schema 的校验比 Anthropic 严格，不合规会让**整个请求** 400，
// 且换账号重试同样失败（历史上的 "Kiro400" 事故即源于此）。
//
// 规则：
//  1. 展开 $ref（Claude Code 的工具 schema 由 zod/pydantic 生成，普遍带 $ref/$defs）
//  2. 删除所有层级的 additionalProperties
//  3. 删除空的 required 数组
//
// 入参不会被修改，返回新的 map。
func SanitizeSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}

	defs := collectDefs(schema)
	ctx := &sanitizeCtx{defs: defs, nodes: 0}
	cleaned := sanitizeValue(schema, ctx, 0)

	out, ok := cleaned.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return out
}

// collectDefs 收集 $defs 与 definitions 下的可引用子 schema。
func collectDefs(schema map[string]any) map[string]any {
	defs := make(map[string]any)
	for _, key := range []string{"$defs", "definitions"} {
		raw, ok := schema[key].(map[string]any)
		if !ok {
			continue
		}
		for name, val := range raw {
			defs[key+"/"+name] = val
		}
	}
	return defs
}

// resolveRef 按 "#/$defs/Name" 形态查表。找不到返回 nil。
func resolveRef(ref string, defs map[string]any) any {
	trimmed := strings.TrimPrefix(ref, "#/")
	if val, ok := defs[trimmed]; ok {
		return val
	}
	return nil
}

// sanitizeValue 递归清洗任意 JSON 值。
func sanitizeValue(value any, ctx *sanitizeCtx, depth int) any {
	switch v := value.(type) {
	case map[string]any:
		// 预算检查放在最前面，且对 $ref 包装节点同样生效——它自己也是即将
		// 产出的一个节点。放在 $ref 分支判断之后会让 $ref 分支的提前 return
		// 绕过检查，预算就只约束了"展开访问次数"而不是"输出节点数"。
		if !ctx.spend() {
			return map[string]any{"type": "object"}
		}

		// $ref 展开：超过深度上限则丢掉 $ref，退化为宽松对象，
		// 保证自引用 schema 能终止。
		if ref, ok := v["$ref"].(string); ok {
			if depth >= maxRefDepth {
				return map[string]any{"type": "object"}
			}
			target := resolveRef(ref, ctx.defs)
			if target == nil {
				// 无法解析的 $ref：退化为宽松对象，好过让上游 400。
				return map[string]any{"type": "object"}
			}
			merged := sanitizeValue(target, ctx, depth+1)
			mergedMap, ok := merged.(map[string]any)
			if !ok {
				return merged
			}
			// $ref 同级的其他关键字（如 description）保留下来，
			// 但同样受预算约束——耗尽就停止合并，不再继续产出节点。
			for key, val := range v {
				if key == "$ref" {
					continue
				}
				if _, exists := mergedMap[key]; exists {
					continue
				}
				if ctx.exhausted() {
					break
				}
				mergedMap[key] = sanitizeValue(val, ctx, depth+1)
			}
			return mergedMap
		}

		out := make(map[string]any, len(v))
		for key, val := range v {
			switch key {
			case "additionalProperties", "$defs", "definitions", "$schema", "$id", "required":
				// required 延后到循环结束后处理（见下）：必须先知道
				// out["properties"] 最终产出了什么，才能过滤掉预算耗尽时
				// 被丢弃的属性名。map range 顺序是随机的，若像其它 key 一样
				// 就地处理，"required" 有时会抢在 "properties" 被预算掐断
				// 之前先落进 out，产出一个引用不存在属性的 required——这正是
				// 本预算机制要防止的 Kiro400 同类风险（结构不一致的 schema）。
				continue
			}
			// 预算耗尽时停止迭代剩余 key，已经产出的 key 保留在 out 里，
			// 不整体丢弃——丢弃已构建好的兄弟节点没有必要：只要不再产出
			// 新容器，预算就已经被守住了；且丢弃会连锁上溯，让本该完整
			// 保留的浅层兄弟节点也被无谓清空。
			if ctx.exhausted() {
				break
			}
			out[key] = sanitizeValue(val, ctx, depth+1)
		}
		if arr, ok := v["required"].([]any); ok && len(arr) > 0 {
			if required := filterRequired(arr, v, out); len(required) > 0 {
				out["required"] = required
			}
		}
		return out

	case []any:
		if !ctx.spend() {
			// 数组的退化 stub 必须是空数组而非对象——在 enum/oneOf/anyOf 之下
			// 用对象替换数组会产出无效 schema，可能触发本函数正要防止的 400。
			return []any{}
		}

		out := make([]any, 0, len(v))
		for _, item := range v {
			// 同 map 分支：耗尽后停止迭代剩余元素，已经产出的元素保留，
			// 不整体丢弃为空数组。
			if ctx.exhausted() {
				break
			}
			out = append(out, sanitizeValue(item, ctx, depth+1))
		}
		return out

	default:
		return value
	}
}

// filterRequired 过滤 required 数组，只保留在输出 properties 里仍然存在的
// 属性名。预算耗尽可能让 properties 整体或部分被丢弃（见 sanitizeValue
// map 分支的注释）；不过滤的话 required 会引用输出里已经不存在的属性，
// 产生结构不一致的 schema，重新引入本预算机制本要防止的 Kiro400 风险。
//
// 只有原始 schema 本就带 "properties" 时才过滤——required 与 properties
// 语义独立，没有本地 properties（例如只靠 patternProperties 约束）的
// schema 不受影响，required 原样保留。
//
// 复制而非别名：返回值不能与入参共享底层数组，与本文件其它地方"入参不会
// 被修改"的约定一致。
func filterRequired(arr []any, original, out map[string]any) []any {
	required := make([]any, len(arr))
	copy(required, arr)

	if _, hadProperties := original["properties"]; !hadProperties {
		return required
	}

	// out["properties"] 可能整体被预算掐断（不存在或非 map，读到 nil），
	// 也可能只是部分属性被掐断（存在但缺了几个 key）——props 是 nil 时，
	// 对它做 map 查找是安全的零值读取，required 里的每个名字都查不到，
	// 结果是 filtered 为空，required 整个被丢弃，与"properties 都没了，
	// required 自然也不该留下"的直觉一致，不需要单独分支处理。
	props, _ := out["properties"].(map[string]any)
	filtered := make([]any, 0, len(required))
	for _, name := range required {
		nameStr, isStr := name.(string)
		if !isStr {
			// 非字符串的 required 元素本身就不合规范，原样透传，
			// 保持与旧行为一致的宽松度，不在这里额外收紧。
			filtered = append(filtered, name)
			continue
		}
		if _, exists := props[nameStr]; exists {
			filtered = append(filtered, name)
		}
	}
	return filtered
}
