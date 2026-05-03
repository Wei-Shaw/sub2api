package plugin

import "strings"

// AuthType 标识插件路由的鉴权类型。
const (
	AuthTypeNone   = "none"
	AuthTypeAPIKey = "apikey"
	AuthTypeUser   = "user"
	AuthTypeAdmin  = "admin"
)

// RouteEntry 表示路由表中一条路由记录,用于将请求转发到对应的插件进程。
type RouteEntry struct {
	Method     string // HTTP 方法,"*" 表示匹配所有方法
	PathPrefix string // 路径前缀,如 "/api/v1/plugins/foo/"
	PluginName string // 关联的插件名称,用于日志/统计
	AuthType   string // 鉴权类型: none / apikey / user / admin
	ProxyURL   string // 反向代理目标地址,如 "http://127.0.0.1:50123"
	IsGateway  bool   // 是否是网关类型(影响请求体处理与流式转发)
}

// RouteTable 是插件路由表的不可变快照。
// 通过 atomic.Pointer 实现无锁原子替换,避免 Match 与 Add/Remove 的并发冲突。
type RouteTable struct {
	entries  []RouteEntry
	byPlugin map[string][]int // plugin name -> entries 索引列表
}

// NewRouteTable 创建一个空的路由表。
func NewRouteTable() *RouteTable {
	return &RouteTable{
		entries:  make([]RouteEntry, 0),
		byPlugin: make(map[string][]int),
	}
}

// Match 根据 method + path 查找匹配的路由条目。
// 优先选择最长（段数最多）前缀匹配,以保证更精确的路由命中。
// PathPrefix 中的 Gin 风格参数段（:id、*rest）被视为通配符,
// 匹配对应位置的任意非空段。
func (t *RouteTable) Match(method, path string) (*RouteEntry, bool) {
	if t == nil || len(t.entries) == 0 {
		return nil, false
	}
	pathSegs := strings.Split(path, "/")
	var (
		best    *RouteEntry
		bestLen int
	)
	for i := range t.entries {
		entry := &t.entries[i]
		if entry.Method != "*" && !strings.EqualFold(entry.Method, method) {
			continue
		}
		if n := matchSegments(pathSegs, entry.PathPrefix); n > bestLen {
			best = entry
			bestLen = n
		}
	}
	return best, best != nil
}

// HasPrefix 用于在 ServeHTTP 中快速判断路径是否可能命中插件路由。
func (t *RouteTable) HasPrefix(path string) bool {
	if t == nil {
		return false
	}
	pathSegs := strings.Split(path, "/")
	for i := range t.entries {
		if matchSegments(pathSegs, t.entries[i].PathPrefix) > 0 {
			return true
		}
	}
	return false
}

// matchSegments 逐段比较 path 是否以 pattern 为前缀。
// pattern 中以 ':' 或 '*' 开头的段视为通配符,匹配任意非空段。
// 尾部斜杠产生的空段会被忽略（"/demo/" 等价于 "/demo" 做前缀匹配）。
// 返回匹配的段数（0 = 不匹配）,用于最长匹配选择。
func matchSegments(pathSegs []string, pattern string) int {
	patSegs := strings.Split(pattern, "/")
	if len(patSegs) > 1 && patSegs[len(patSegs)-1] == "" {
		patSegs = patSegs[:len(patSegs)-1]
	}
	if len(pathSegs) < len(patSegs) {
		return 0
	}
	for i, ps := range patSegs {
		if len(ps) > 0 && (ps[0] == ':' || ps[0] == '*') {
			if pathSegs[i] == "" {
				return 0
			}
			continue
		}
		if ps != pathSegs[i] {
			return 0
		}
	}
	return len(patSegs)
}

// AddPlugin 返回一个新的路由表,将指定插件的路由追加到现有条目末尾。
// 原表保持不变,符合不可变快照的设计。
// 如果同名插件已经存在路由,先剔除旧的再追加新的,实现"更新"语义。
func (t *RouteTable) AddPlugin(name string, entries []RouteEntry) *RouteTable {
	base := t
	if _, exists := t.byPlugin[name]; exists {
		base = t.RemovePlugin(name)
	}

	out := NewRouteTable()
	out.entries = make([]RouteEntry, 0, len(base.entries)+len(entries))
	out.entries = append(out.entries, base.entries...)
	for k, idxs := range base.byPlugin {
		copied := make([]int, len(idxs))
		copy(copied, idxs)
		out.byPlugin[k] = copied
	}

	startIdx := len(out.entries)
	out.entries = append(out.entries, entries...)

	idxs := make([]int, 0, len(entries))
	for i := startIdx; i < len(out.entries); i++ {
		idxs = append(idxs, i)
		out.entries[i].PluginName = name
	}
	if len(idxs) > 0 {
		out.byPlugin[name] = idxs
	}
	return out
}

// RemovePlugin 返回一个新的路由表,其中已剔除指定插件的所有路由。
func (t *RouteTable) RemovePlugin(name string) *RouteTable {
	out := NewRouteTable()
	if t == nil || len(t.entries) == 0 {
		return out
	}

	skipIdx := make(map[int]struct{})
	if idxs, ok := t.byPlugin[name]; ok {
		for _, i := range idxs {
			skipIdx[i] = struct{}{}
		}
	}

	out.entries = make([]RouteEntry, 0, len(t.entries))
	// 保留旧索引到新索引的映射,用于重建 byPlugin。
	oldToNew := make(map[int]int, len(t.entries))
	for i, e := range t.entries {
		if _, skip := skipIdx[i]; skip {
			continue
		}
		oldToNew[i] = len(out.entries)
		out.entries = append(out.entries, e)
	}

	for plugin, idxs := range t.byPlugin {
		if plugin == name {
			continue
		}
		newIdxs := make([]int, 0, len(idxs))
		for _, oldIdx := range idxs {
			if newIdx, ok := oldToNew[oldIdx]; ok {
				newIdxs = append(newIdxs, newIdx)
			}
		}
		if len(newIdxs) > 0 {
			out.byPlugin[plugin] = newIdxs
		}
	}
	return out
}
