package service

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Codex 账号绑定时区：把出站请求体 input 里由客户端生成的 environment_context 时区与
// 日期改写成账号级恒定值。
//
// 多人共享一个账号时，各下游客户端按本机生成时区与当天日期，上游看到同一账号在多个
// 时区之间跳动；绑定后每个账号只呈现一个时区。这是提示词正文而不是请求头，因此与设备
// 指纹收敛（openai_codex_fingerprint.go）相互独立：绑定即生效，不依赖指纹档位。
//
// 改写规则：
//   - 只改已存在的标签，绝不新增。请求体不含 environment_context 时原样返回。
//   - 日期不是简单替换成「账号时区的今天」：历史轮次的 environment_context 会随全量
//     历史一起重发（store=false），把旧日期一律改成今天会篡改历史。这里按「账号时区
//     今天减客户端时区今天」的天数差整体平移，旧块保持相对关系不变，同一请求内的新块
//     恰好落在账号时区的今天。客户端时区缺失或无法加载时只改时区不动日期。
//   - 幂等：同值二次改写结果相同，重试链路把改写过的载荷再送进来不会漂移。
const (
	codexEnvironmentContextOpenTag = "<environment_context>"
	codexEnvironmentDateLayout     = "2006-01-02"

	// codexTimezoneExtraKey 是账号 Extra 里的 IANA 时区名，空值表示不绑定。
	codexTimezoneExtraKey = "codex_timezone"

	// codexTimezoneNameMaxLen 是可接受的时区名长度上限（最长的 IANA 名约 32 字符）。
	codexTimezoneNameMaxLen = 64
	// codexEnvironmentLocationCacheMax 是时区缓存条目上限。时区名来自客户端请求体，
	// 即使只缓存能加载的名字，"Asia/./Shanghai" 这类路径变体也能无限制造新 key。
	codexEnvironmentLocationCacheMax = 256
)

var (
	codexEnvironmentTimezonePattern = regexp.MustCompile(`<timezone>([^<]*)</timezone>`)
	codexEnvironmentDatePattern     = regexp.MustCompile(`<current_date>(\d{4}-\d{2}-\d{2})</current_date>`)

	// time.LoadLocation 每次都读 zoneinfo，热路径上缓存已解析结果。
	codexEnvironmentLocationCache     sync.Map // string -> *time.Location
	codexEnvironmentLocationCacheSize atomic.Int64
)

// loadCodexLocationCached 解析并缓存 IANA 时区。只缓存加载成功的名字且条目数有上限，
// 失败结果不缓存：客户端可发送任意字符串，缓存失败结果会让内存无限增长。
func loadCodexLocationCached(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > codexTimezoneNameMaxLen {
		return nil
	}
	if cached, ok := codexEnvironmentLocationCache.Load(name); ok {
		loc, _ := cached.(*time.Location)
		return loc
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	// ponytail: 软上限，并发未命中可能略微超出；缓存满后新名字每次都走 LoadLocation。
	if codexEnvironmentLocationCacheSize.Load() < codexEnvironmentLocationCacheMax {
		// Clone：name 可能是请求体文本的子串，直接作 key 会让整段文本常驻内存。
		if _, loaded := codexEnvironmentLocationCache.LoadOrStore(strings.Clone(name), loc); !loaded {
			codexEnvironmentLocationCacheSize.Add(1)
		}
	}
	return loc
}

// NormalizeCodexTimezone 校验 IANA 时区名，非法值返回空串。管理端写入前调用，
// 避免把加载不了的名字存进账号，出站时再静默失效。
func NormalizeCodexTimezone(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || loadCodexLocationCached(name) == nil {
		return ""
	}
	return name
}

// GetCodexTimezone 返回账号绑定的 IANA 时区名，未绑定或值非法时返回空串。
func (a *Account) GetCodexTimezone() string {
	if a == nil || !a.UsesOpenAICodexProtocol() {
		return ""
	}
	return NormalizeCodexTimezone(a.GetExtraString(codexTimezoneExtraKey))
}

// applyCodexEnvironmentContextTimezone 按账号绑定时区改写请求体 input 里所有
// environment_context 文本。账号未绑定时区、或请求体不含 environment_context 时原样返回。
func applyCodexEnvironmentContextTimezone(account *Account, body []byte, now time.Time) []byte {
	timezone := account.GetCodexTimezone()
	if timezone == "" || len(body) == 0 {
		return body
	}
	loc := loadCodexLocationCached(timezone)
	if loc == nil {
		return body
	}
	// 不能在原始字节上预检开标签：请求体由 encoding/json 重编后，尖括号会落地成转义形式。
	// 只在 gjson 解码后的文本上判断。
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body
	}
	accountNow := now.In(loc)
	input.ForEach(func(itemIndex, item gjson.Result) bool {
		content := item.Get("content")
		switch {
		case content.Type == gjson.String:
			if rewritten, changed := rewriteCodexEnvironmentContextText(content.String(), timezone, accountNow); changed {
				body = setCodexJSONString(body, fmt.Sprintf("input.%d.content", itemIndex.Int()), rewritten)
			}
		case content.IsArray():
			content.ForEach(func(partIndex, part gjson.Result) bool {
				text := part.Get("text")
				if text.Type != gjson.String {
					return true
				}
				if rewritten, changed := rewriteCodexEnvironmentContextText(text.String(), timezone, accountNow); changed {
					body = setCodexJSONString(body, fmt.Sprintf("input.%d.content.%d.text", itemIndex.Int(), partIndex.Int()), rewritten)
				}
				return true
			})
		}
		return true
	})
	return body
}

func setCodexJSONString(body []byte, path, value string) []byte {
	updated, err := sjson.SetBytes(body, path, value)
	if err != nil {
		return body
	}
	return updated
}

// rewriteCodexEnvironmentContextText 改写一段文本里的时区与日期标签。文本不含
// environment_context 时不动；含有时才扫描标签（同一文本里可能嵌着多份，例如记忆整理
// 提示词引用的历史轮次，统一改写以保持一致）。
func rewriteCodexEnvironmentContextText(text, timezone string, accountNow time.Time) (string, bool) {
	if !strings.Contains(text, codexEnvironmentContextOpenTag) {
		return text, false
	}
	changed := false

	// 客户端时区取自文本里第一个 timezone 标签；平移天数只在该时区可加载时计算。
	dayDelta, haveDelta := 0, false
	if match := codexEnvironmentTimezonePattern.FindStringSubmatch(text); match != nil {
		if clientLoc := loadCodexLocationCached(match[1]); clientLoc != nil {
			clientToday := truncateCodexDay(accountNow.In(clientLoc))
			dayDelta = int(truncateCodexDay(accountNow).Sub(clientToday).Hours() / 24)
			haveDelta = true
		}
	}

	rewritten := codexEnvironmentTimezonePattern.ReplaceAllStringFunc(text, func(tag string) string {
		if codexEnvironmentTimezonePattern.FindStringSubmatch(tag)[1] == timezone {
			return tag
		}
		changed = true
		return "<timezone>" + timezone + "</timezone>"
	})
	if haveDelta && dayDelta != 0 {
		rewritten = codexEnvironmentDatePattern.ReplaceAllStringFunc(rewritten, func(tag string) string {
			raw := codexEnvironmentDatePattern.FindStringSubmatch(tag)[1]
			parsed, err := time.ParseInLocation(codexEnvironmentDateLayout, raw, time.UTC)
			if err != nil {
				return tag
			}
			changed = true
			return "<current_date>" + parsed.AddDate(0, 0, dayDelta).Format(codexEnvironmentDateLayout) + "</current_date>"
		})
	}
	return rewritten, changed
}

// truncateCodexDay 把时刻截到所在时区的当日零点，再换成 UTC 表达以便相减得到整天差。
func truncateCodexDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
