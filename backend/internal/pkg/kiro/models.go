package kiro

import (
	"regexp"
	"strings"
)

// defaultKiroModel 是 Kiro 目前最主力的型号，只用作 DefaultModels() 兜底
// 列表里的一个已知条目（供测试断言"列表非空且包含一个合理默认值"）。
// **不是 MapModel 的兜底目标**——不支持的模型名直接拒绝（ok=false），
// 不会静默换成这个值再假装请求成功。
const defaultKiroModel = "claude-sonnet-4.6"

// kiroModelAliases 把 Anthropic 风格的模型名映射到 Kiro 上游名。
// Kiro 大多数型号用点号版本号（claude-sonnet-4.6），Anthropic 客户端用
// 连字符。
//
// I5 的原判断"Kiro 没有 Opus 型号"是错的——真实账号测试证实 claude-opus-5
// 确实被 Kiro 支持，Kiro 用不分次版本号的 "claude-opus-5" 原样服务它。
//
// 历史教训：曾经参考一个第三方 Kiro 代理实现的 map_model 表，一并加过
// opus-4.5/4.6/4.7/4.8、sonnet-5、fable-5/fable-5-1，其中 fable 那两条
// 被真实账号测试证伪——Kiro 对 "claude-fable-5" 回真实 400
// INVALID_MODEL_ID，说明当时那个第三方参考实现不可靠，遂把同一来源加入
// 的其余条目（opus-4.5/4.6/4.7/4.8、sonnet-5）一并移除，只留下真正验证
// 过的 claude-opus-5。
//
// 2026-09-06 更新：opus-4.5/4.6/4.7/4.8、sonnet-5 这次是被真正证实支持
// 的——不是靠猜或第三方参考实现，而是用真实账号的 access_token 直接调用
// Kiro 自己的权威接口 ListAvailableModels（POST
// https://management.<region>.kiro.dev/，X-Amz-Target:
// AmazonCodeWhispererService.ListAvailableModels，需要 profileArn，见
// KiroOAuthService.DiscoverProfileArn）拿到的账号真实模型清单，逐条比对
// 后加回来的，跟上一次"参考第三方实现"的性质完全不同——这是 Kiro 自己
// 权威声明的"这个账号能用哪些模型"，不是猜测。fable-5/fable-5-1 在这份
// 真实清单里确实不存在，维持拒绝，与之前真实账号测试的结论一致。
//
// ListAvailableModels 返回的清单里还有一批非 Claude 系模型（gpt-5.6-sol/
// terra/luna、deepseek-3.2、minimax-m2.5/m2.1、glm-5、qwen3-coder-next）。
// 2026-09-06 第二次更新：其中 gpt-5.6-sol/terra/luna 这三个已经收录（见下方
// 别名表），理由：(1) 账号权威清单 ListAvailableModels 确认存在；
// (2) 计费天然免费——PricingService.GetModelPricing 是全局按模型名查表，
// 不分平台，"gpt-" 前缀已有现成的兜底匹配（matchOpenAIModel），不需要为
// Kiro 单独建价；(3) 参考开源实现 kiro2cc-proxy（真实可用、有 CI 和针对
// 这几个模型的回归测试）交叉印证过这三个模型名，且它们走的是跟 Claude
// 系模型同一条 Anthropic 协议管线，没有发现需要我们这边跟着改的协议层
// 差异——我们自己没有发送 additionalModelRequestFields 这类字段（"假
// 思考"是纯 prompt 注入），kiro2cc-proxy 因为发了这个字段被 gpt-5.6-*
// 拒成 400 的坑，天然不适用于我们。deepseek-3.2、minimax-m2.5/m2.1、
// glm-5、qwen3-coder-next 这 5 个依然不在收录范围：没有现成计价条目，
// 接入前先得决定计费怎么算，留给后续单独评估。
//
// 白名单机制本身没有问题（架构对齐 AntigravityGatewayService 的
// DefaultAntigravityModelMapping，见 MapModel 文档）——第一次出错是内容
// 来源不够可靠（未经验证的第三方参考实现），不是机制设计的问题。以后
// 新增条目：优先用 ListAvailableModels 这类账号自己的权威接口验证（比
// "测试连接"更直接，不需要真的发一次聊天请求去猜），退而求其次可以用
// 真实账号测试连接直接验证候选模型名（点号原生形态可以不经别名表直接
// 透传，见 MapModel 规则 2），拿到确认后再收录进别名表，不要只凭第三方
// 参考实现下结论。
var kiroModelAliases = map[string]string{
	"claude-sonnet-4":   "claude-sonnet-4",
	"claude-sonnet-4-5": "claude-sonnet-4.5",
	"claude-sonnet-4-6": "claude-sonnet-4.6",
	"claude-haiku-4-5":  "claude-haiku-4.5",
	"claude-opus-5":     "claude-opus-5",
	"claude-sonnet-5":   "claude-sonnet-5",
	"claude-opus-4-5":   "claude-opus-4.5",
	"claude-opus-4-6":   "claude-opus-4.6",
	"claude-opus-4-7":   "claude-opus-4.7",
	"claude-opus-4-8":   "claude-opus-4.8",
	// 非 Claude 系，点号原生形态本身就等于请求形态——kiroNativeName 的
	// 透传正则只认 "claude-" 前缀，这三个必须显式收录才能透传，跟
	// claude-opus-5/claude-sonnet-5（同样无小数点、regex 也不认）是同一
	// 个道理。
	"gpt-5.6-sol":   "gpt-5.6-sol",
	"gpt-5.6-terra": "gpt-5.6-terra",
	"gpt-5.6-luna":  "gpt-5.6-luna",
}

// dateSuffix 匹配 Anthropic 模型名尾部的日期版本，如 -20250929。
var dateSuffix = regexp.MustCompile(`-\d{8}$`)

// kiroNativeName 匹配已经是 Kiro 形态的名字（版本号带点）。
var kiroNativeName = regexp.MustCompile(`^claude-[a-z]+-\d+\.\d+$`)

// MapModel 把客户端请求的模型名转换为 Kiro 上游可识别的名字。
//
// 设计对齐 AntigravityGatewayService 的既有约定（domain.
// DefaultAntigravityModelMapping + Account.GetMappedModel）：本地维护一份
// 尽量准确、可维护的白名单，命中就映射，未命中就干净拒绝——不是"猜一个
// 默认模型硬答应"，也不是"不管三七二十一转发给上游让它兜底"。这个仓库里
// 已经有稳定先例（Antigravity/Grok 都是这个模式），Kiro 没有理由另起一条
// 不同的路。
//
// kiroModelAliases 白名单内容出过错（第一次是把不认识的名字静默换成
// sonnet-4.6 再假装成功；第二次是漏收了 Kiro 实际支持的 claude-opus-5；
// 第三次是信了一个第三方参考实现里未经验证就加了几条、其中 fable 那两条
// 被真实测试证伪——见上面的详细说明），但错误从未出在"要不要维护白
// 名单"这个机制本身。每一条都只应该在真实验证（账号自己的权威接口
// ListAvailableModels 确认在清单里、真实账号测试连接得到 200，或已是
// 长期批量验证过的既有条目）之后才收录。
//
// 规则按优先级：
//  1. 空输入 → ok=false
//  2. "auto" 或已是 Kiro 点号形态（claude-xxx-N.M）→ 原样透传，ok=true
//     （上游新增型号无需改代码；是否真的存在交给上游判定）
//  3. 命中别名表 → 映射，ok=true
//  4. 去掉日期后缀后命中别名表 → 映射，ok=true
//  5. 其余 → 不支持，ok=false（调用方必须拒绝，不能静默换模型或裸转发）
func MapModel(requested string) (mapped string, ok bool) {
	name := strings.ToLower(strings.TrimSpace(requested))
	if name == "" {
		return "", false
	}

	if name == "auto" || kiroNativeName.MatchString(name) {
		return name, true
	}

	if m, found := kiroModelAliases[name]; found {
		return m, true
	}

	if stripped := dateSuffix.ReplaceAllString(name, ""); stripped != name {
		if m, found := kiroModelAliases[stripped]; found {
			return m, true
		}
	}

	return "", false
}

// DefaultModels 返回未从上游拉到模型清单时对外暴露的兜底列表——覆盖
// kiroModelAliases 里全部已确认受支持的目标型号（保持与其同步，新增别名
// 时这里也要加，否则 /v1/models 兜底列表会缺新模型）。
func DefaultModels() []string {
	return []string{
		"claude-sonnet-4.6",
		"claude-sonnet-4.5",
		"claude-sonnet-4",
		"claude-haiku-4.5",
		"claude-opus-5",
		"claude-sonnet-5",
		"claude-opus-4.8",
		"claude-opus-4.7",
		"claude-opus-4.6",
		"claude-opus-4.5",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
	}
}
