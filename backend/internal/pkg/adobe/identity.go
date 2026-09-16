// Package adobe 实现 Adobe Firefly 直连协议：IMS cookie 换 token、带浏览器指纹的
// 3p 端点提交、异步轮询、产物下载。
//
// 应用身份（client_id / Origin / scope / x-api-key）以 Firefly 前端
// （firefly.adobe.com / clio-playground-web）的抓包为准，不是 Adobe Express。
// TLS / User-Agent 仍走 Chrome 伪装，与抓包所用的 Camoufox Firefox 无关。
//
// 本包对 sub2api 内部零依赖：只产出类型化错误，错误到账号状态机的映射由 service 层负责。
package adobe

// Identity 收敛 Firefly 直连所需的全部身份参数。
//
// 这些值在上游侧是一组互相印证的整体：Adobe 的风控会同时校验 TLS 指纹、User-Agent
// 与 sec-ch-ua，三者必须描述同一个浏览器版本。把它们放进同一个结构体，是为了让
// 「UA 说 Chrome 145、TLS 指纹却是 Chrome 124」这类不一致在 code review 里显形——
// adobe2api 就有这个缺陷（core/adobe_client.py 的 impersonate=chrome124 配
// Chrome/145.0.0.0 的 UA）。
//
// 修改任何一项前先确认其余项是否需要同步变更。
type Identity struct {
	// IMSRefreshURL 是 cookie 换 access_token 的端点。
	IMSRefreshURL string
	// IMSClientID 是 IMS 表单里的 client_id。
	IMSClientID string
	// IMSScope 是 IMS 表单里的 scope。
	IMSScope string

	// FireflyAPIKey 是 firefly-3p 端点的 x-api-key。
	FireflyAPIKey string
	// CreditsAPIKey 是 credits/balance 端点的 x-api-key，与 FireflyAPIKey 不同。
	CreditsAPIKey string

	// Origin / Referer 需与 IMSClientID 对应的前端站点一致。
	Origin  string
	Referer string

	// UserAgent 与 SecChUA 必须描述同一浏览器版本，且与 TLSProfile 对应。
	UserAgent string
	SecChUA   string
	// SecChUAPlatform 是 sec-ch-ua-platform 头，需与 UserAgent 里的平台一致。
	SecChUAPlatform string

	// TLSProfile 是 bogdanfinn/tls-client 的 profile 名，需与 UserAgent 的 Chrome
	// 版本对应。取值见 profiles.MappedTLSClients 的键。
	TLSProfile string
}

// DefaultIdentity 是当前在用的身份参数（Adobe Firefly Web 前端）。
//
// client_id / Origin / scope / jslVersion 对齐 firefly.adobe.com 抓包
// （/tmp/adobe-capture-20260910-060909Z 中成功换出 token 的那次 IMS）。
// UA / sec-ch-ua / TLSProfile 仍是一组 Chrome 伪装，不要改成抓包里的 Firefox。
var DefaultIdentity = Identity{
	IMSRefreshURL: "https://adobeid-na1.services.adobe.com/ims/check/v6/token?jslVersion=v2-v0.54.0-3-g58cfcb7",
	IMSClientID:   "clio-playground-web",
	IMSScope: "AdobeID,firefly_api,openid,pps.read,pps.write," +
		"additional_info.projectedProductContext,additional_info.ownerOrg," +
		"uds_read,uds_write,ab.manage,read_organizations,additional_info.roles," +
		"account_cluster.read,creative_production,tk_platform,tk_platform_sync,profile",

	FireflyAPIKey: "clio-playground-web",
	CreditsAPIKey: "SunbreakWebUI1",

	Origin:  "https://firefly.adobe.com",
	Referer: "https://firefly.adobe.com/",

	UserAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	SecChUA:         `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`,
	SecChUAPlatform: `"Windows"`,

	// chrome_146 配 Chrome/145 的 UA 差一个小版本。本轮只换应用身份，TLS 三件套
	// 维持已验证过的 Chrome 组合，不跟 Camoufox 的 Firefox/152 对齐。
	TLSProfile: "chrome_146",
}

// Firefly 直连使用的上游端点。
const (
	ImageSubmitURL = "https://firefly-3p.ff.adobe.io/v2/3p-images/generate-async"
	VideoSubmitURL = "https://firefly-3p.ff.adobe.io/v2/3p-videos/generate-async"
	ImageUploadURL = "https://firefly-3p.ff.adobe.io/v2/storage/image"
	CreditsURL     = "https://firefly.adobe.io/v1/credits/balance"
)

// profileURLs 是账号信息端点，按顺序尝试直到某个返回可用数据。
var profileURLs = []string{
	"https://ims-na1.adobelogin.com/ims/profile/v1",
	"https://adobeid-na1.services.adobe.com/ims/profile/v1",
}

// withDefaults 用 DefaultIdentity 补齐零值字段，便于调用方只覆盖个别项。
func (i Identity) withDefaults() Identity {
	d := DefaultIdentity
	if i.IMSRefreshURL == "" {
		i.IMSRefreshURL = d.IMSRefreshURL
	}
	if i.IMSClientID == "" {
		i.IMSClientID = d.IMSClientID
	}
	if i.IMSScope == "" {
		i.IMSScope = d.IMSScope
	}
	if i.FireflyAPIKey == "" {
		i.FireflyAPIKey = d.FireflyAPIKey
	}
	if i.CreditsAPIKey == "" {
		i.CreditsAPIKey = d.CreditsAPIKey
	}
	if i.Origin == "" {
		i.Origin = d.Origin
	}
	if i.Referer == "" {
		i.Referer = d.Referer
	}
	if i.UserAgent == "" {
		i.UserAgent = d.UserAgent
	}
	if i.SecChUA == "" {
		i.SecChUA = d.SecChUA
	}
	if i.SecChUAPlatform == "" {
		i.SecChUAPlatform = d.SecChUAPlatform
	}
	if i.TLSProfile == "" {
		i.TLSProfile = d.TLSProfile
	}
	return i
}
