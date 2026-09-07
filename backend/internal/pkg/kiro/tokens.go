package kiro

import (
	"encoding/json"
	"math"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// EstimateText 估算一段文本的 token 数。
//
// Kiro 的 meteringEvent 只给 credits 和真实 cache token，不给 input/output token，
// 而本仓库按 token 计费，因此这两项必须本地估算。公式移植自
// Kiro-Go/proxy/token_estimator.go，按字符类加权。经验误差 ±10-20%。
func EstimateText(s string) int {
	if s == "" {
		return 0
	}

	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return 0
	}
	if n < 5 {
		if est := int(math.Ceil(float64(n) / 3.0)); est > 1 {
			return est
		}
		return 1
	}

	var ascii, digits, symbols, nonASCII int
	for _, r := range runes {
		switch {
		case r >= 0x80:
			nonASCII++
		case r >= '0' && r <= '9':
			digits++
		case (r >= '!' && r <= '/') || (r >= ':' && r <= '@') ||
			(r >= '[' && r <= '`') || (r >= '{' && r <= '~'):
			symbols++
		default:
			ascii++
		}
	}

	est := int(math.Ceil(
		float64(ascii)/4.5 +
			float64(digits)/2.0 +
			float64(symbols)/1.5 +
			float64(nonASCII)/1.5,
	))
	if est < 1 {
		return 1
	}
	return est
}

// imageTokenEstimate 是单张图片计入 input token 的固定近似值。
//
// Anthropic 官方公式是 tokens ≈ (宽 px × 高 px) / 750，一张常见的 1092x1092
// 图片约 1590 token，取整近似为 1600。Kiro 请求里图片以 base64 内联在
// source.data，字节数和像素面积并不成正比（同样分辨率，PNG 可能比 JPEG 大
// 好几倍），拿字节数去估算 token 完全失真——一张几 MB 的截图能把整条请求的
// 估算 input token 顶到几十万，严重偏离真实计费。改用一个固定近似值代替，
// 量级贴近典型图片，且与图片编码格式、字节数无关。
const imageTokenEstimate = 1600

// EstimateRequestInput 估算整个请求的 input token：system + 全部消息 + 工具声明。
func EstimateRequestInput(req *apicompat.AnthropicRequest) int {
	if req == nil {
		return 0
	}

	total := 0

	// FlattenSystem 的错误在此被有意吞掉：本函数只做计费近似，前提是调用方
	// 已经在别处（真正走请求转换路径时）用同一个 FlattenSystem 校验过 system
	// 字段能被正确解析——届时错误会被那条路径处理并中断请求。这里即便解析
	// 失败也只是把这部分估算为 0，不影响整体估算的可用性，不该因为一个
	// 计费近似函数而重复报错或让调用方多处理一次错误。
	if system, err := FlattenSystem(req.System); err == nil {
		total += EstimateText(system)
	}

	for _, m := range req.Messages {
		total += estimateContentTokens(m.Content)
	}

	for _, tool := range req.Tools {
		total += EstimateText(tool.Name)
		total += EstimateText(tool.Description)
		total += EstimateText(string(tool.InputSchema))
	}

	return total
}

// estimateContentTokens 估算一段消息 content（string 或 content block 数组）
// 的 input token。
//
// 解析失败时退回整段原始 JSON 字符串估算：这样不会因某条消息/某个块解析
// 失败就让整体估算归零，牺牲一点精确度换稳健性——EstimateText 本身也只是
// ±10-20% 的近似，不追求精确计费。
func estimateContentTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return EstimateText(asString)
	}

	// 逐块保留原始 JSON：apicompat.AnthropicContentBlock 只建模了
	// text/thinking/image/tool_use/tool_result 这几种类型的字段，
	// document、web_search_tool_result、server_tool_use、mcp_tool_use 等
	// 类型解析进结构体后关心的字段全是零值——若在丢弃原始 JSON 之后才按
	// 类型分派，这些块会被系统性少算成 0（真实历史消息里从未出现的样子）。
	// 保留每个块自己的 raw，让默认分支能退回该块自身估算，而不是整个
	// content 数组的估算全部归零。
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw, &rawBlocks); err != nil {
		// 既不是字符串也不是合法的 block 数组：退回原始字符串估算。
		return EstimateText(string(raw))
	}

	total := 0
	for _, rb := range rawBlocks {
		total += estimateBlockTokens(rb)
	}
	return total
}

// estimateBlockTokens 估算单个 content block 的 input token。
func estimateBlockTokens(raw json.RawMessage) int {
	var b apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &b); err != nil {
		return EstimateText(string(raw))
	}

	switch b.Type {
	case "image":
		// 只计固定近似值，绝不触碰 b.Source.Data（见 imageTokenEstimate 文档）。
		return imageTokenEstimate
	case "tool_use":
		return EstimateText(b.Name) + EstimateText(string(b.Input))
	case "tool_result":
		// tool_result.Content 同样可能是 string 或 block 数组，且数组里
		// 同样可能夹带 image 块（工具把图片结果传回模型），必须递归走
		// 同一条估算路径，不能直接字符串化。
		return EstimateText(b.ToolUseID) + estimateContentTokens(b.Content)
	case "thinking":
		// Signature 是多轮对话里客户端连同 Thinking 正文一起回传的加密
		// 签名字段，二者同属这个块的内容；只算 Thinking 会在带 Signature
		// 的多轮历史里系统性少算。
		return EstimateText(b.Thinking) + EstimateText(b.Signature)
	case "text":
		return EstimateText(b.Text)
	default:
		// redacted_thinking 的实际载荷在 AnthropicContentBlock 未建模的
		// "data" 字段；document / web_search_tool_result / server_tool_use /
		// mcp_tool_use 等新块类型同样未建模。这些类型解析进结构体后关心的
		// 字段是零值，退回该块自身的原始 JSON 估算，好过假装它不存在而记
		// 成 0——与顶层"整段解析失败退回原始字符串"是同一个稳健性原则，
		// 只是把粒度收窄到单个块，不再连累同一 content 数组里其它块的估算。
		return EstimateText(string(raw))
	}
}
