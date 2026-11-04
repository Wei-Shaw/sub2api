package domain

import (
	"strconv"
	"strings"
)

// GroupLocaleText 是分组名称与描述在某个界面语言下的译文。
// 字段为空表示该语言沿用分组自身的 name/description。
type GroupLocaleText struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// GroupI18n 以界面语言代码（如 "en"、"zh"）为键保存分组展示文案的译文。
// 仅用于页面展示；分组的唯一性、搜索与各类日志仍以 name 为准。
type GroupI18n map[string]GroupLocaleText

// Localize 返回 name、description 在请求首选语言下的版本；该语言没有译文的字段保持原值。
// acceptLanguage 是 Accept-Language 请求头。只采用其中优先级最高的语言：原字段的语言
// 未知，退到次选语言的译文不一定比原字段更合适。
func (m GroupI18n) Localize(acceptLanguage, name, description string) (string, string) {
	text, ok := m.lookup(preferredLanguage(acceptLanguage))
	if !ok {
		return name, description
	}
	if text.Name != "" {
		name = text.Name
	}
	if text.Description != "" {
		description = text.Description
	}
	return name, description
}

// LocalizeName 是只需要名称时的 Localize。
func (m GroupI18n) LocalizeName(acceptLanguage, name string) string {
	name, _ = m.Localize(acceptLanguage, name, "")
	return name
}

// CanonicalLocale 按 BCP 47 的惯用大小写书写语言代码：主语言小写，4 位文字代码首字母大写，
// 2 位地区代码大写，其余小写（zh-hant-tw -> zh-Hant-TW）；单字母扩展标记之后的部分全部小写。
// 只调整大小写，不校验代码是否合法。
func CanonicalLocale(locale string) string {
	subtags := strings.Split(locale, "-")
	extension := false
	for i, subtag := range subtags {
		subtag = strings.ToLower(subtag)
		switch {
		case i == 0 || extension:
		case len(subtag) == 1:
			extension = true
		case len(subtag) == 2:
			subtag = strings.ToUpper(subtag)
		case len(subtag) == 4:
			subtag = strings.ToUpper(subtag[:1]) + subtag[1:]
		}
		subtags[i] = subtag
	}
	return strings.Join(subtags, "-")
}

// lookup 先按完整语言代码匹配（zh-TW），再按主语言匹配（zh-CN -> zh），不区分大小写。
// 写入时语言代码已统一为 CanonicalLocale，同一语言只有一个键；若存有仅大小写不同的多个键，
// 规范写法的键优先，其余按字节序取最小者，保证同一请求语言的结果固定。
func (m GroupI18n) lookup(tag string) (GroupLocaleText, bool) {
	if tag == "" || len(m) == 0 {
		return GroupLocaleText{}, false
	}
	primary, _, _ := strings.Cut(tag, "-")
	for _, candidate := range []string{tag, primary} {
		candidate = CanonicalLocale(candidate)
		if text, ok := m[candidate]; ok {
			return text, true
		}
		matched := ""
		for locale := range m {
			if strings.EqualFold(locale, candidate) && (matched == "" || locale < matched) {
				matched = locale
			}
		}
		if matched != "" {
			return m[matched], true
		}
	}
	return GroupLocaleText{}, false
}

// preferredLanguage 取 Accept-Language 中权重最高的语言代码，权重相同取靠前者；
// 通配符 * 与 q=0 的条目不参与。
func preferredLanguage(acceptLanguage string) string {
	best, bestQ := "", 0.0
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		if value, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		if q > bestQ {
			best, bestQ = tag, q
		}
	}
	return best
}
