package service

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// generationRedactionVersion 记录脱敏规则版本，写入 ai_generation_content.redaction_version。
// 任何脱敏规则变更都应 bump 此值，便于后续审计/回填判断某行经过了哪套规则。
// v2（D3-a）：在 email/phone 之外补齐 身份证 / 银行卡 / 高熵 opaque token 脱敏。
const generationRedactionVersion = 2

var (
	// rePIIEmail 匹配常见 email。
	rePIIEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// rePIIPhone 匹配常见电话格式，同时避免误伤标准 UUID / 长纯数字 request-id（B.1 Codex 复核标黄）。
	// 三个分支：国际格式（带 +）｜分隔分组（空格/短横线/点，且分隔符非数字，强制至少 3 组）｜中国手机（1[3-9] 起 11 位）。
	// 均不含换行（避免跨行吞并）；长度锚 + \b 使 12 位 UUID 末段、16 位纯数字 id、模型名（claude-opus-4-8）均不命中。
	rePIIPhone = regexp.MustCompile(`\+\d[\d().\-\t ]{6,16}\d|\b\d{2,4}[.\-\t ]\d{3,4}[.\-\t ]\d{3,4}\b|\b1[3-9]\d{9}\b`)

	// reCNIDCard 中国大陆居民身份证号候选（18 位：17 位数字 + 校验位 0-9/X）。仅作候选；
	// 是否真为身份证由 isValidCNID 二次校验（出生日期窗 + ISO 7064 mod-11-2 校验位），
	// 以免误伤 18-19 位 snowflake / 订单号。不做 15 位旧式（纯 \d{15} 会误伤等长订单号）。
	reCNIDCard = regexp.MustCompile(`\b\d{17}[0-9Xx]\b`)
	// reCardCandidate 银行卡号候选：13-19 位，允许单个空格/短横线分组分隔。仅作候选；
	// 是否真为卡号由 Luhn 校验决定（见 redactGenerationStructuredPII 回调），
	// 以免误伤等长非 Luhn 纯数字 id（如 16 位非 Luhn 订单号）。
	reCardCandidate = regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`)
)

// redactGenerationPII 在结构化/密钥脱敏之后，补一层自由文本 PII（email/电话）脱敏。
// 现有 logredact / content_moderation 脱敏均不覆盖自由文本 PII，这里补齐缺口。
func redactGenerationPII(s string) string {
	if s == "" {
		return s
	}
	s = rePIIEmail.ReplaceAllString(s, "[EMAIL]")
	s = rePIIPhone.ReplaceAllString(s, "[PHONE]")
	return s
}

// redactGenerationStructuredPII 补齐 email/phone 之外的结构化 PII / 高熵凭据脱敏（D3-a）：
//
//	① 身份证号（候选正则 + 出生日期窗 + ISO 7064 校验位，避免误伤 snowflake/订单号）→ [ID]
//	② 银行卡号（候选正则 + 去分隔 + 长度 13-19 + Luhn，避免误伤等长非 Luhn 数字 id）→ [CARD]
//	③ 高熵不带前缀 opaque token（复用 video 同包 shape-aware 工具）→ [已脱敏]
//
// 子 pass 顺序 ID → 卡 → opaque：先打类型标记，泛化扫尾只看残余。每条都是 \b 锚定候选正则 +
// 回调校验（RE2 无 lookahead/backreference，校验必须在 ReplaceAllStringFunc 里做），
// 因此不会像早期过宽电话正则那样把 UUID/标识符切碎。
func redactGenerationStructuredPII(s string) string {
	if s == "" {
		return s
	}
	s = reCNIDCard.ReplaceAllStringFunc(s, func(tok string) string {
		if isValidCNID(tok) {
			return "[ID]"
		}
		return tok
	})
	s = reCardCandidate.ReplaceAllStringFunc(s, func(tok string) string {
		digits := stripCardSeparators(tok)
		if isLuhnValid(digits) {
			return "[CARD]"
		}
		return tok
	})
	// videoOpaqueTokenCandidate / looksLikeOpaqueVideoSecret 同在 service 包（见
	// video_gateway_redact.go）：捕获 20+ 位无分隔、同时含数字与字母的高熵串，
	// 纯字母词 / 纯数字 id 留存。命名为历史遗留（最早落地在 video 网关），逻辑通用。
	s = videoOpaqueTokenCandidate.ReplaceAllStringFunc(s, func(tok string) string {
		if looksLikeOpaqueVideoSecret(tok) {
			return "[已脱敏]"
		}
		return tok
	})
	return s
}

// stripCardSeparators 去掉银行卡候选里的分组分隔符（空格/短横线），只留数字，供 Luhn 校验。
func stripCardSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= '0' && c <= '9' {
			_ = b.WriteByte(c)
		}
	}
	return b.String()
}

// isLuhnValid 对 13-19 位纯数字串做 Luhn 校验。区间外或含非数字直接判否，
// 使 16 位非 Luhn 订单号（如测试夹具 1234567890123456，Luhn 和=64）不被当作卡号。
func isLuhnValid(num string) bool {
	if len(num) < 13 || len(num) > 19 {
		return false
	}
	sum := 0
	alt := false
	for i := len(num) - 1; i >= 0; i-- {
		c := num[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// isValidCNID 校验 18 位中国大陆身份证号：出生日期窗（位 7-14 = YYYYMMDD，年 1900..当年、
// 月 1-12、日 1-31）+ ISO 7064 mod-11-2 校验位。两道都过才算身份证，
// 使 18-19 位 snowflake / 订单号（日期窗或校验位不过）不被误抹。
func isValidCNID(id string) bool {
	if len(id) != 18 {
		return false
	}
	for i := 0; i < 17; i++ {
		if c := id[i]; c < '0' || c > '9' {
			return false
		}
	}
	year, _ := strconv.Atoi(id[6:10])
	month, _ := strconv.Atoi(id[10:12])
	day, _ := strconv.Atoi(id[12:14])
	if year < 1900 || year > time.Now().Year() {
		return false
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	weights := [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checks := [11]byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(id[i]-'0') * weights[i]
	}
	want := checks[sum%11]
	got := id[17]
	if got == 'x' {
		got = 'X'
	}
	return got == want
}

// redactGenerationPrompt 对请求体（JSON）做入库前脱敏：
// 结构化敏感键(RedactJSON) → 自由文本 PII(email/phone) → 结构化 PII/高熵凭据(身份证/卡/opaque) → 兜底密钥/Token 模式(content_moderation)。
func redactGenerationPrompt(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	out := logredact.RedactJSON(body)
	out = redactGenerationPII(out)
	out = redactGenerationStructuredPII(out)
	out = redactContentModerationSecrets(out)
	return out
}

// redactGenerationResponse 对响应抽样（SSE/JSON 文本）做入库前脱敏：
// 轻量文本脱敏(RedactText) → 自由文本 PII(email/phone) → 结构化 PII/高熵凭据(身份证/卡/opaque) → 兜底密钥/Token 模式。
func redactGenerationResponse(sample []byte) string {
	if len(sample) == 0 {
		return ""
	}
	out := logredact.RedactText(string(sample))
	out = redactGenerationPII(out)
	out = redactGenerationStructuredPII(out)
	out = redactContentModerationSecrets(out)
	return out
}
