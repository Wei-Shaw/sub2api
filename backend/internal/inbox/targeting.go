package inbox

import (
	"encoding/json"
	"fmt"
)

// 广播 targeting 是一个 JSON 表达式，描述"哪些用户命中该广播"。求值发生在 catchup
// 读取阶段（fan-out on read），由服务端在应用层对候选广播逐条 Match(userAttrs)。
//
// 支持的算子（op）：
//
//	{"op":"all_users"}                                   // 匹配所有用户
//	{"op":"equals","attr":"plan","value":"pro"}          // 属性等值
//	{"op":"in","attr":"plan","values":["pro","team"]}    // 属性属于集合
//	{"op":"and","clauses":[ <expr>, <expr>, ... ]}       // 全部命中
//	{"op":"or","clauses":[ <expr>, <expr>, ... ]}        // 任一命中
//
// 表达式可递归嵌套，但限制最大深度防止恶意构造。
const (
	opAllUsers = "all_users"
	opEquals   = "equals"
	opIn       = "in"
	opAnd      = "and"
	opOr       = "or"

	// maxTargetingDepth 限制嵌套深度，防止深层递归导致栈溢出 / DoS。
	maxTargetingDepth = 8
)

// targetingNode 是 targeting 表达式解析后的语法树节点。
type targetingNode struct {
	Op      string           `json:"op"`
	Attr    string           `json:"attr,omitempty"`
	Value   any              `json:"value,omitempty"`
	Values  []any            `json:"values,omitempty"`
	Clauses []*targetingNode `json:"clauses,omitempty"`
}

// Targeting 是解析并校验通过后的广播定向表达式，可安全地对用户属性求值。
type Targeting struct {
	root *targetingNode
}

// ParseTargeting 解析并校验 targeting JSON。校验失败返回 ErrInvalidTargeting（带 cause）。
func ParseTargeting(raw json.RawMessage) (*Targeting, error) {
	if len(raw) == 0 {
		return nil, ErrInvalidTargeting.WithCause(fmt.Errorf("targeting 为空"))
	}
	var root targetingNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, ErrInvalidTargeting.WithCause(err)
	}
	if err := validateNode(&root, 0); err != nil {
		return nil, ErrInvalidTargeting.WithCause(err)
	}
	return &Targeting{root: &root}, nil
}

// ValidateTargeting 仅校验 targeting 合法性，不保留解析结果。用于发布时的入参检查。
func ValidateTargeting(raw json.RawMessage) error {
	_, err := ParseTargeting(raw)
	return err
}

// validateNode 递归校验单个节点的结构完整性。
func validateNode(n *targetingNode, depth int) error {
	if depth > maxTargetingDepth {
		return fmt.Errorf("targeting 嵌套深度超过 %d", maxTargetingDepth)
	}
	switch n.Op {
	case opAllUsers:
		return nil
	case opEquals:
		if n.Attr == "" {
			return fmt.Errorf("equals 缺少 attr")
		}
		if n.Value == nil {
			return fmt.Errorf("equals 缺少 value")
		}
		return nil
	case opIn:
		if n.Attr == "" {
			return fmt.Errorf("in 缺少 attr")
		}
		if len(n.Values) == 0 {
			return fmt.Errorf("in 的 values 不能为空")
		}
		return nil
	case opAnd, opOr:
		if len(n.Clauses) == 0 {
			return fmt.Errorf("%s 的 clauses 不能为空", n.Op)
		}
		for _, c := range n.Clauses {
			if c == nil {
				return fmt.Errorf("%s 存在空子句", n.Op)
			}
			if err := validateNode(c, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("未知 op: %q", n.Op)
	}
}

// Match 判断给定用户属性是否命中该 targeting 表达式。
//
// attrs 为用户属性 map（键为属性名，值可为 string / bool / 数值）。数值比较统一归一化
// 为 float64，因此 JSON 的 float64 与 Go 的 int/int64 可正确互比。
func (t *Targeting) Match(attrs map[string]any) bool {
	if t == nil || t.root == nil {
		return false
	}
	return evalNode(t.root, attrs)
}

func evalNode(n *targetingNode, attrs map[string]any) bool {
	switch n.Op {
	case opAllUsers:
		return true
	case opEquals:
		actual, ok := attrs[n.Attr]
		if !ok {
			return false
		}
		return valuesEqual(actual, n.Value)
	case opIn:
		actual, ok := attrs[n.Attr]
		if !ok {
			return false
		}
		for _, v := range n.Values {
			if valuesEqual(actual, v) {
				return true
			}
		}
		return false
	case opAnd:
		for _, c := range n.Clauses {
			if !evalNode(c, attrs) {
				return false
			}
		}
		return true
	case opOr:
		for _, c := range n.Clauses {
			if evalNode(c, attrs) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// valuesEqual 归一化后比较两个值。数值统一转 float64，其余按类型直接比较。
func valuesEqual(a, b any) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			return af == bf
		}
		return false
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	default:
		return false
	}
}

// toFloat 尝试把任意数值类型归一化为 float64。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
