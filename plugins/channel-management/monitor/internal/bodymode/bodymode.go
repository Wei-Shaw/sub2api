// Package bodymode 提供 channel-monitor 自定义请求体模式的归一化常量与函数。
//
// 这是一个最小依赖的工具包：service 与 repository 都从这里 import，避免在两侧
// 各自维护一份 defaultBodyMode 实现导致语义漂移（参见 T13 审核：
// repository/channel_monitor_repo.go 和 service/template_service.go 曾各自实现
// 一份近似函数）。
//
// 取值定义与 ent enum / DB CHECK 约束保持一致。
package bodymode

const (
	// Off 使用 adapter 默认 body（忽略 BodyOverride）。
	Off = "off"
	// Merge adapter 默认 body 与 BodyOverride 浅合并（用户优先；
	// model/messages/contents 等关键字段在 checker 黑名单内会被静默丢弃）。
	Merge = "merge"
	// Replace 完全用 BodyOverride 作为 body；跳过 challenge 校验，
	// 改成 HTTP 2xx + 响应非空即视为可用（用户负责构造 body）。
	Replace = "replace"
)

// Normalize 将空串归一为 Off。其它值原样返回（合法性由 validate 层负责）。
//
// service 写入 / repository 读出 / handler dto 转换都必须经过此函数，
// 这样空串与 "off" 永远视作同义，避免下游 switch 漏 case。
func Normalize(mode string) string {
	if mode == "" {
		return Off
	}
	return mode
}
