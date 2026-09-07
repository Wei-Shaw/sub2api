package kiro

import (
	"bytes"
)

// Signal 是对一次上游响应的语义分类，决定调度侧的动作。
//
// 分类结果通过 Retryable / Failoverable 把「能不能重试、能不能换账号」编码进类型，
// 而不是交给每个调用点自行判断 —— 设计文档 §7.2 记录的两次事故都源于误判。
type Signal int

const (
	// SignalOK 表示成功。
	SignalOK Signal = iota
	// SignalAuthExpired 表示 token 失效，应刷新后重试一次。
	SignalAuthExpired
	// SignalOverage 表示 overage 未开启或已超上限。这是当前账号订阅设置的问题，
	// 不是请求本身的问题——账号池里开了 overage 或还没到上限的其它账号可以
	// 正常服务同一个请求，因此应当换账号（Failoverable）。
	SignalOverage
	// SignalRateLimited 表示该端点额度耗尽，应先换端点。
	SignalRateLimited
	// SignalNetworkRegion 表示网络/区域问题（典型是 INVALID_MODEL_ID）。
	// 这不是账号的错，绝不能据此禁用账号。Retryable=true 但问题根源是请求
	// 命中的区域/网络路径本身（例如大陆直连必现 INVALID_MODEL_ID），换端点
	// 重试大概率仍会复现；调用方必须给这个信号单独设一个重试上限，不能当作
	// 普通瞬时错误无限重试（I7）。
	SignalNetworkRegion
	// SignalBadRequest 表示我们自己构造的请求不合法。
	// 不可重试、不可换账号 —— 换了一样失败。
	SignalBadRequest
	// SignalSuspended 表示订阅被停用或 profile 不可用，应禁用账号。禁用账号
	// 和"当前这次请求要不要换账号重试"是两件独立的事——禁用账号防止之后的
	// 请求继续路由到它，但账号池里没被停用的其它账号完全可以正常处理同一个
	// 请求，因此应当换账号（Failoverable），不能因为要禁用当前账号就连带
	// 让当前请求整体失败。
	SignalSuspended
	// SignalCreditsExhausted 表示账号额度耗尽，应冷却并换账号。
	SignalCreditsExhausted
	// SignalUnknown 是兜底（含 5xx）。
	SignalUnknown
)

// String 返回稳定的短名，用于日志与告警检索。改动会破坏既有检索。
func (s Signal) String() string {
	switch s {
	case SignalOK:
		return "ok"
	case SignalAuthExpired:
		return "auth_expired"
	case SignalOverage:
		return "overage"
	case SignalRateLimited:
		return "rate_limited"
	case SignalNetworkRegion:
		return "network_region"
	case SignalBadRequest:
		return "bad_request"
	case SignalSuspended:
		return "suspended"
	case SignalCreditsExhausted:
		return "credits_exhausted"
	default:
		return "unknown"
	}
}

// Retryable 表示是否值得就当前账号再试一次（可能换端点）。
func (s Signal) Retryable() bool {
	switch s {
	case SignalAuthExpired, SignalRateLimited, SignalNetworkRegion, SignalUnknown:
		return true
	default:
		return false
	}
}

// Failoverable 表示是否应该换一个账号重试。
//
// 判断标准是"问题出在当前账号，还是出在请求/网络本身"：
//   - 出在当前账号（额度耗尽、订阅停用、overage 未开、鉴权失效）—— 池子里
//     其它账号大概率没有这个问题，换账号有机会成功，恒为 true。
//   - 出在请求或网络本身（格式错误、区域/网络问题）—— 换哪个账号结果都
//     一样，换账号只会把整池配额烧光，恒为 false。
//
// SignalBadRequest 恒为 false：请求本身不合法，换账号只会把整池配额烧光。
// SignalNetworkRegion 恒为 false：网络问题与账号无关，换账号无济于事。
// SignalSuspended / SignalOverage 恒为 true：这两者都是当前账号的订阅/配置
// 问题，池子里其它账号完全可能没有同样的问题；是否禁用当前账号是另一套
// 独立机制，不能因为要禁用账号就连带让当前请求也失败。
func (s Signal) Failoverable() bool {
	switch s {
	case SignalRateLimited, SignalCreditsExhausted, SignalAuthExpired, SignalUnknown,
		SignalSuspended, SignalOverage:
		return true
	default:
		return false
	}
}

// 错误 body 中的特征串。全部小写比较。
var (
	invalidModelIDMarkers = [][]byte{
		[]byte("invalid_model_id"),
		[]byte("invalid model id"),
	}
	suspensionMarkers = [][]byte{
		// 用具体短语而不是裸 "suspend"：裸子串会命中回显文本或助手原文里任何
		// 提到 suspend 的地方，误伤健康账号。
		[]byte("has been suspended"),
		[]byte("account is suspended"),
		[]byte("subscription is suspended"),
		[]byte("account is disabled"),
		[]byte("profile is not available"),
		[]byte("profilearn is not available"),
	}
	creditsExhaustedMarkers = [][]byte{
		[]byte("credits exhausted"),
		[]byte("insufficient credits"),
		[]byte("not enough credits"),
		[]byte("monthly request limit"),
		[]byte("usage limit reached"),
	}
)

// classifyMarkers 依次用 invalidModelIDMarkers / creditsExhaustedMarkers /
// suspensionMarkers 检查一段已转小写的文本，命中即返回对应 Signal；
// 全不命中返回 (SignalOK, false)，SignalOK 在这里只是占位，调用方必须看
// ok 而不是这个零值。抽出来是因为 Classify 和 ClassifyUpstreamError 都要
// 按同一套特征串、同一个优先级顺序匹配——INVALID_MODEL_ID 必须先于额度/
// 停用判定，额度耗尽必须先于停用判定，两条路径不能各自维护一份顺序。
func classifyMarkers(lower []byte) (Signal, bool) {
	if len(lower) == 0 {
		return SignalOK, false
	}
	if containsAny(lower, invalidModelIDMarkers) {
		return SignalNetworkRegion, true
	}
	if containsAny(lower, creditsExhaustedMarkers) {
		return SignalCreditsExhausted, true
	}
	if containsAny(lower, suspensionMarkers) {
		return SignalSuspended, true
	}
	return SignalOK, false
}

func containsAny(haystack []byte, needles [][]byte) bool {
	for _, n := range needles {
		if bytes.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// Classify 把一次上游响应归类。
//
// 检查顺序是有意为之：
//  1. 2xx 必须最先判定为 SignalOK，绝不允许被 body 内容反过来改判 —— 成功响应
//     的 body 里可能回显了请求文本或助手原文，其中出现敏感词不代表账号出了问题。
//  2. 非 2xx 时，body 特征优先于状态码。INVALID_MODEL_ID 通常伴随 400
//     返回，若先按状态码判成 SignalBadRequest，就会掩盖「这其实是网络问题」这一事实。
func Classify(status int, body []byte) Signal {
	if status >= 200 && status < 300 {
		return SignalOK
	}

	lower := bytes.ToLower(bytes.TrimSpace(body))

	// 网络/区域问题必须最先识别 —— 它伪装成 400。
	if sig, ok := classifyMarkers(lower); ok {
		return sig
	}

	switch status {
	case 401, 403:
		return SignalAuthExpired
	case 402:
		return SignalOverage
	case 429:
		return SignalRateLimited
	case 400:
		return SignalBadRequest
	default:
		return SignalUnknown
	}
}

// ClassifyUpstreamError 把流内异常帧（HTTP 200 之下的 exception 帧）归类为
// Signal（I6）。
//
// exception 帧和 Classify 处理的「带状态码 + body」的响应是两条完全独立的
// 路径：Kiro 用 HTTP 200 起流，真正的错误（限流、鉴权失效、
// INVALID_MODEL_ID 等）会作为流内的 exception 帧出现，此时已经没有状态码
// 可用。修复前 stream.go 的 handle() 只是把 exception 帧包成
// *UpstreamError 原样往上抛，从未经过任何分类——调度侧拿到的是一个不知道
// 能不能重试、能不能换账号的裸错误，只能靠 errors.As 拿到具体字段自己再
// 判断一遍，等于每个调用点各写一套（还可能各写出不一致的一套）。
//
// 检查顺序与 Classify 保持一致（复用同一个 classifyMarkers）：Message 里的
// 特征串优先于异常类型本身——AWS event-stream 的 ValidationException 常常
// 伴随 INVALID_MODEL_ID，若先按类型判成 SignalBadRequest，会掩盖
// "这其实是网络问题" 这一事实。
// 已知缺口（整分支复核发现，parked）：不同于 Classify 有 HTTP 402 → SignalOverage
// 这条明确映射，这里没有 overage 的 Message 特征串或 Type 分支。设计文档只把
// overage 系在状态码 402 上（getUsageLimits.overageConfiguration 才是权威来源，
// 见 spec §「402 | overage 未开启或超上限」），没有任何证据表明 overage 会以流内
// exception 帧的形式出现——Kiro 用 HTTP 200 起流，若 overage 检查发生在流开始
// 之前，它只会是一次 402 响应，走 Classify 而不会到这里。留空而不是猜一个未经
// 验证的特征串：猜错比不猜更危险（会把真正匹配的信号错误吸收掉）。若 C 组
// Task 16/17 接线阶段用真实账号验证到 overage 确实会以流内异常出现，在此补上
// 对应分支，并把这行注释一并删掉。
func ClassifyUpstreamError(err *UpstreamError) Signal {
	if err == nil {
		return SignalUnknown
	}

	lower := bytes.ToLower(bytes.TrimSpace([]byte(err.Message)))
	if sig, ok := classifyMarkers(lower); ok {
		return sig
	}

	switch err.Type {
	case "ThrottlingException", "TooManyRequestsException":
		return SignalRateLimited
	case "AccessDeniedException", "UnauthorizedException", "UnrecognizedClientException", "ExpiredTokenException":
		return SignalAuthExpired
	case "ValidationException", "InvalidRequestException", "SerializationException", "BadRequestException":
		return SignalBadRequest
	default:
		return SignalUnknown
	}
}
