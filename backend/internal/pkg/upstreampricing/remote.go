package upstreampricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Remote struct{ Client *http.Client }

func NewRemote(proxyURL string) *Remote {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		if p, err := url.Parse(proxyURL); err == nil {
			t.Proxy = http.ProxyURL(p)
		}
	}
	return &Remote{Client: &http.Client{Transport: t, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return safeError("定价接口不接受跳转") }}}
}

func ValidateConfig(c *Config) error {
	if c.Provider != "newapi" && c.Provider != "sub2api" {
		return safeError("请选择 New API 或 Sub2API 定价来源")
	}
	u, err := url.Parse(strings.TrimSpace(c.BaseURL))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return safeError("面板地址无效，请填写不含凭证和查询参数的站点地址")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return safeError("面板地址需使用 HTTPS；本机回环地址可使用 HTTP")
	}
	c.BaseURL = strings.TrimRight(u.String(), "/")
	if len(c.BaseURL) > 2048 || len(c.Token) > 16384 || strings.ContainsAny(c.Token, "\r\n") {
		return safeError("地址或凭证格式无效")
	}
	if len(c.Group) > 120 || len(c.GroupID) > 120 || len(c.UserID) > 32 || strings.ContainsAny(c.UserID, "\r\n") {
		return safeError("分组或站点用户 ID 无效")
	}
	if c.Provider == "newapi" && c.UserID != "" {
		for _, ch := range c.UserID {
			if ch < '0' || ch > '9' {
				return safeError("New API 用户 ID 必须是数字")
			}
		}
	}
	if c.IntervalMinutes == 0 {
		c.IntervalMinutes = 60
	}
	if c.IntervalMinutes != 15 && c.IntervalMinutes != 60 && c.IntervalMinutes != 360 && c.IntervalMinutes != 1440 {
		return safeError("同步间隔仅支持 15、60、360 或 1440 分钟")
	}
	if len(c.Overrides) > 1000 {
		return safeError("人工覆盖数量超出限制")
	}
	for _, p := range c.Overrides {
		if err := ValidatePrice(p); err != nil {
			return err
		}
	}
	return nil
}

func (r *Remote) get(ctx context.Context, c Config, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, safeError("无法构造定价查询")
	}
	if c.Token == "" {
		return nil, safeError("需要面板读取凭证；模型调用 Key 不一定具有读取权限")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sub2API-Fork-Pricing/1")
	if c.Provider == "sub2api" {
		req.Header.Set("X-User-UI-Request", "1")
	}
	if c.Provider == "newapi" && c.UserID != "" {
		req.Header.Set("New-Api-User", c.UserID)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, safeError("连接定价接口失败、超时或发生跳转，请检查面板地址与网络")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, safeError(fmt.Sprintf("定价接口 HTTP %d：请检查凭证、权限和站点版本", resp.StatusCode))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4000001))
	if err != nil || len(b) > 4000000 {
		return nil, safeError("定价接口响应过大或读取失败")
	}
	var v map[string]any
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.UseNumber()
	if d.Decode(&v) != nil || v == nil {
		return nil, safeError("站点未返回兼容的 JSON，可能是登录页或验证页")
	}
	if ok, present := v["success"].(bool); present && !ok {
		return nil, safeError("站点拒绝查询，请检查面板凭证与分组权限")
	}
	if code, present := v["code"]; present && fmt.Sprint(code) != "0" && fmt.Sprint(code) != "200" {
		return nil, safeError("站点拒绝查询，请检查面板凭证与接口版本")
	}
	return v, nil
}

func stringValue(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return string(s)
	}
	return ""
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func validName(s string) bool {
	return strings.TrimSpace(s) == s && s != "" && len(s) <= 300 && !strings.ContainsAny(s, "\r\n\x00")
}

func (r *Remote) Groups(ctx context.Context, c Config) ([]Group, error) {
	if c.Provider == "newapi" {
		v, err := r.get(ctx, c, "/api/user/self/groups")
		if err != nil {
			return nil, err
		}
		m := object(v["data"])
		if m == nil || len(m) > 1000 {
			return nil, safeError("New API 用户分组结构不兼容")
		}
		out := []Group{}
		for name, value := range m {
			if !validName(name) {
				return nil, safeError("站点分组名无效")
			}
			ratio, err := number(object(value)["ratio"])
			if err != nil {
				return nil, err
			}
			s := ""
			if ratio != nil {
				s = *ratio
			}
			out = append(out, Group{Name: name, Multiplier: s})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}
	v, err := r.get(ctx, c, "/api/v1/groups/available?timezone=Asia%2FShanghai")
	if err != nil {
		return nil, err
	}
	rates, err := r.get(ctx, c, "/api/v1/groups/rates")
	if err != nil {
		return nil, err
	}
	a, ok := v["data"].([]any)
	rm := object(rates["data"])
	if !ok || len(a) > 1000 || rm == nil {
		return nil, safeError("Sub2API 用户分组或倍率结构不兼容")
	}
	out := []Group{}
	seen := map[string]bool{}
	for _, item := range a {
		g := object(item)
		name := stringValue(g["name"])
		id := stringValue(g["id"])
		if !validName(name) || seen[name] || id == "" {
			return nil, safeError("站点分组名重复或 ID 无效")
		}
		seen[name] = true
		raw := rm[id]
		if raw == nil {
			raw = g["rate_multiplier"]
		}
		ratio, err := number(raw)
		if err != nil || ratio == nil {
			return nil, safeError("缺少有效的分组或用户专属倍率")
		}
		out = append(out, Group{ID: id, Name: name, Multiplier: *ratio})
	}
	return out, nil
}

func (r *Remote) Fetch(ctx context.Context, c Config) (Catalog, error) {
	groups, err := r.Groups(ctx, c)
	if err != nil {
		return Catalog{}, err
	}
	var selected *Group
	for i := range groups {
		if groups[i].Name == c.Group && (c.GroupID == "" || groups[i].ID == c.GroupID) {
			if selected != nil {
				return Catalog{}, safeError("分组不唯一")
			}
			selected = &groups[i]
		}
	}
	if selected == nil {
		return Catalog{}, safeError("所选分组不在该用户可用分组中，请重新读取分组")
	}
	var cat Catalog
	if c.Provider == "newapi" {
		v, err := r.get(ctx, c, "/api/pricing")
		if err != nil {
			return cat, err
		}
		cat, err = NormalizeNewAPI(v, *selected)
		if err != nil {
			return cat, err
		}
	} else {
		v, err := r.get(ctx, c, "/api/v1/model-plaza")
		if err != nil {
			return cat, err
		}
		cat, err = NormalizeSub2API(v, *selected)
		if err != nil {
			return cat, err
		}
	}
	if err := finalize(&cat, c.Overrides); err != nil {
		return Catalog{}, err
	}
	return cat, nil
}

func NormalizeNewAPI(v map[string]any, g Group) (Catalog, error) {
	a, ok := v["data"].([]any)
	if !ok || len(a) > 10000 {
		return Catalog{}, safeError("New API 模型价格目录结构不兼容")
	}
	if g.Multiplier == "" {
		p, err := number(object(v["group_ratio"])[g.Name])
		if err != nil || p == nil {
			return Catalog{}, safeError("缺少可验证的上游分组倍率")
		}
		g.Multiplier = *p
	}
	if _, err := decimal(g.Multiplier); err != nil {
		return Catalog{}, err
	}
	cat := Catalog{Group: g, Models: []Price{}, RemoteVersion: stringValue(v["pricing_version"])}
	for _, row := range a {
		m := object(row)
		allowed, ok := m["enable_groups"].([]any)
		if !ok {
			return Catalog{}, safeError("模型未声明可用分组")
		}
		enabled := false
		for _, name := range allowed {
			if name == g.Name || name == "all" {
				enabled = true
			}
		}
		if !enabled {
			continue
		}
		p := Price{Model: stringValue(m["model_name"]), Mode: "unavailable"}
		if !validName(p.Model) {
			return cat, safeError("模型名无效")
		}
		if len(array(m["billing_plugin_variants"])) > 0 || m["billing_usage_schema"] != nil {
			p.Reason = "包含尚未适配的插件或额外计费维度"
		} else if mode := stringValue(m["billing_mode"]); mode != "" && mode != "ratio" && !(mode == "per_request" && stringValue(m["quota_type"]) == "1") {
			p.Reason = "尚未适配表达式、阶梯或其他计价模式"
		} else {
			var err error
			switch stringValue(m["quota_type"]) {
			case "1":
				p.Mode = "per_request"
				p.Request, err = number(m["model_price"])
			case "0":
				p.Mode = "token"
				p.Input, err = scaled(m["model_ratio"], 2)
				if err == nil && p.Input != nil {
					base, _ := decimal(*p.Input)
					dest := []**string{&p.Output, &p.CacheRead, &p.CacheWrite}
					keys := []string{"completion_ratio", "cache_ratio", "create_cache_ratio"}
					for i, key := range keys {
						var ratio *string
						ratio, err = number(m[key])
						if err != nil {
							break
						}
						if ratio != nil {
							r, _ := decimal(*ratio)
							product := new(big.Rat).Mul(base, r)
							s := format(product, 18)
							rounded, parseErr := decimal(s)
							if parseErr != nil || rounded.Cmp(product) != 0 {
								err = safeError("换算后的单价精度或范围超出支持范围")
								break
							}
							*dest[i] = &s
						}
					}
				}
			default:
				p.Reason = "站点未声明支持的计价类型"
			}
			if err != nil || ValidatePrice(p) != nil {
				p.Mode = "unavailable"
				p.Reason = "缺少有效单价或必要倍率"
			}
		}
		if p.Mode == "unavailable" && p.Reason == "" {
			p.Reason = "暂不支持此计价规则"
		}
		cat.Models = append(cat.Models, p)
	}
	if err := finalize(&cat, nil); err != nil {
		return cat, err
	}
	return cat, nil
}

func NormalizeSub2API(v map[string]any, g Group) (Catalog, error) {
	a, ok := object(v["data"])["groups"].([]any)
	if !ok || len(a) > 1000 {
		return Catalog{}, safeError("Sub2API 模型广场结构不兼容")
	}
	var selected map[string]any
	for _, item := range a {
		m := object(item)
		if stringValue(m["name"]) == g.Name && (g.ID == "" || stringValue(m["id"]) == g.ID) {
			if selected != nil {
				return Catalog{}, safeError("模型广场分组不唯一")
			}
			selected = m
		}
	}
	if selected == nil {
		return Catalog{}, safeError("站点未公布所选分组的模型价格；未导入其他分组")
	}
	models, ok := selected["models"].([]any)
	if !ok || len(models) > 10000 {
		return Catalog{}, safeError("所选分组模型目录结构不兼容")
	}
	if _, err := decimal(g.Multiplier); err != nil {
		return Catalog{}, err
	}
	cat := Catalog{Group: g, Models: []Price{}, RemoteVersion: "sub2api-model-plaza"}
	for _, row := range models {
		m := object(row)
		p := Price{Model: stringValue(m["name"]), Mode: "unavailable"}
		if !validName(p.Model) {
			return cat, safeError("模型名无效")
		}
		pricing := object(m["pricing"])
		switch {
		case pricing == nil:
			p.Reason = "站点未提供实付单价，参考价格未自动代用"
		case selected["peak_rate_enabled"] == true:
			p.Reason = "此分组有峰谷倍率，尚未适配"
		case m["time_pricing"] != nil || pricing["time_pricing"] != nil:
			p.Reason = "此模型有分时计价，尚未适配"
		case len(array(pricing["intervals"])) > 0:
			p.Reason = "此模型有上下文区间，需要核对阶梯语义后适配"
		default:
			unsupported := false
			for _, v := range object(pricing["reasoning_effort_multipliers"]) {
				n, err := number(v)
				if err != nil || n == nil || *n != "1" {
					unsupported = true
				}
			}
			if unsupported {
				p.Reason = "此模型有推理强度倍率，尚未适配"
				break
			}
			p.Mode = stringValue(pricing["billing_mode"])
			var err error
			if p.Mode == "per_request" {
				p.Request, err = number(pricing["per_request_price"])
			} else if p.Mode == "token" {
				dest := []**string{&p.Input, &p.Output, &p.CacheRead, &p.CacheWrite, &p.CacheWrite1h, &p.CacheWrite30m, &p.CacheWrite12h}
				keys := []string{"input_price", "output_price", "cache_read_price", "cache_write_price", "cache_write_1h_price", "cache_write_30m_price", "cache_write_12h_price"}
				for i, key := range keys {
					*dest[i], err = scaled(pricing[key], 1000000)
					if err != nil {
						break
					}
				}
				// Sub2API's OpenAI price card uses cache_write_price for 30m;
				// it exposes the separate 12h override explicitly.
				if stringValue(m["platform"]) == "openai" && p.CacheWrite30m == nil {
					p.CacheWrite30m = p.CacheWrite
				}
			}
			if err != nil || ValidatePrice(p) != nil {
				p.Mode = "unavailable"
				p.Reason = "缺少有效单价，或模型使用图片、视频等尚未适配的计价模式"
			}
		}
		cat.Models = append(cat.Models, p)
	}
	if err := finalize(&cat, nil); err != nil {
		return cat, err
	}
	return cat, nil
}
