package plugin

import (
	"fmt"
	"strings"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// HTTP route 命名空间常量与 manifest 路由验证 / 表构建
//
// 核心保留 /api/v1/admin/* 给 host admin handler;
// gateway 路径 /v1/* 需要 http.register.gateway capability;
// 插件自有命名空间 /api/v1/plugin/<name>/* 与 /plugins/<name>/*
// 默认放行(隐式 capability).
//
// 这些前缀字面量同时被 router_middleware.go / route_table.go 使用,
// 但因为字符串值早已定型 (协议级 contract), 这里集中只是为了让
// manager 内部 grep 可读 — 不强行交叉 import 避免循环依赖。
// =============================================================

const (
	// pluginNamespacePrefixAPIv1 是插件可隐式注册的"API 风格"前缀,
	// 当前 hello-world / channel-management 都用这个。
	pluginNamespacePrefixAPIv1 = "/api/v1/plugin/"
	// pluginNamespacePrefixLegacy 是 route_table.go 注释中保留的
	// 兼容前缀,用于早期版本的插件路由声明。
	pluginNamespacePrefixLegacy = "/plugins/"
	// adminReservedPrefix 是 host 永久保留的 admin 命名空间; 即便
	// 插件持有所有 capability 也不允许在此注册 (P12·B-1)。
	adminReservedPrefix = "/api/v1/admin/"
	// pluginAssetsURLPrefix 是核心反向代理插件 entry.js / entry.css
	// 的对外 URL 前缀, 由 FrontendServer 的 manifest 注入逻辑使用。
	pluginAssetsURLPrefix = "/api/v1/plugin-assets/"

	// capabilityHTTPRegisterGateway 是允许插件在共享 gateway 路径
	// (典型 /v1/*) 上注册的 capability canonical 名。复制自
	// pluginsdkroot.CapabilityRegistry, 这里硬编码字符串以避免在 gate
	// 检查中绕远到 SDK 包。
	capabilityHTTPRegisterGateway = "http.register.gateway"
)

// pluginOwnNamespacePrefixes returns the path prefixes a plugin may
// register handlers under without declaring http.register.gateway. The
// canonical form is "/api/v1/plugin/<name>/" (singular plugin), used by
// hello-world / channel-management today; "/plugins/<name>/" is recognised
// as an alternative form for forward compatibility with the route table
// comment in route_table.go.
//
// The two trailing prefixes — "/api/v1/<name>/" and "/api/v1/admin/<name>/"
// — let plugins keep the same URLs they served before being lifted out of
// the host monolith, so external integrations (Stripe Dashboard webhook
// URLs, Alipay/WeChat merchant notifyUrl fields) keep working without any
// operator action. The admin variant is namespaced by plugin name in the
// second segment so it cannot shadow host-owned admin routes like
// /api/v1/admin/users — see validatePluginRoutePaths for the matching
// host-reserved guard.
func pluginOwnNamespacePrefixes(pluginName string) []string {
	return []string{
		pluginNamespacePrefixAPIv1 + pluginName + "/",
		pluginNamespacePrefixLegacy + pluginName + "/",
		"/api/v1/" + pluginName + "/",
		adminReservedPrefix + pluginName + "/",
	}
}

// validatePluginRoutePaths runs the P12·B-1 HTTP namespace gate over a
// plugin's manifest. Each declared GatewayEndpoint / PluginEndpoint path
// must satisfy one of:
//
//  1. starts with "/api/v1/plugin/<plugin>/" or "/plugins/<plugin>/" —
//     the plugin's own namespace, always allowed
//     (CapabilityHTTPRegisterPlugin is default-grant)
//  2. is a host-reserved admin path (/api/v1/admin/*) — ALWAYS denied,
//     even if the plugin holds every capability the host knows about
//  3. otherwise the plugin must hold http.register.gateway, granting it
//     the right to register on shared gateway paths (typically /v1/*)
//
// approvedCaps is the post-normalisation list returned by approveCapabilities;
// it only contains canonical dotted-lowercase names.
func validatePluginRoutePaths(pluginName string, manifest *pluginsdk.ManifestResponse, approvedCaps []string) error {
	hasGatewayCap := false
	for _, c := range approvedCaps {
		if c == capabilityHTTPRegisterGateway {
			hasGatewayCap = true
			break
		}
	}
	ownPrefixes := pluginOwnNamespacePrefixes(pluginName)
	isOwn := func(path string) bool {
		for _, p := range ownPrefixes {
			if strings.HasPrefix(path, p) || path == strings.TrimSuffix(p, "/") {
				return true
			}
		}
		return false
	}
	check := func(path string) error {
		if path == "" {
			return nil
		}
		// Own namespace (any of the four prefixes returned above) is
		// always allowed. Check this BEFORE the admin-reserved guard so
		// /api/v1/admin/<name>/* lands as own-namespace, not as
		// host-reserved — the per-plugin second segment is the
		// namespace boundary that keeps two plugins (or a plugin and
		// the host) from colliding on /api/v1/admin/<other>/...
		if isOwn(path) {
			return nil
		}
		if strings.HasPrefix(path, adminReservedPrefix) {
			return fmt.Errorf("plugin %q cannot register admin path %q (host-reserved)", pluginName, path)
		}
		if !hasGatewayCap {
			return fmt.Errorf("plugin %q registers path %q outside own namespace; declare capability %q in manifest", pluginName, path, capabilityHTTPRegisterGateway)
		}
		return nil
	}
	for _, ep := range manifest.GetGatewayEndpoints() {
		if err := check(ep.GetPath()); err != nil {
			return err
		}
	}
	for _, ep := range manifest.GetPluginEndpoints() {
		if err := check(ep.GetPath()); err != nil {
			return err
		}
	}
	return nil
}

// buildRouteEntries 把 manifest 中声明的 endpoint 转换成 RouteEntry。
// gateway_endpoints 与 plugin_endpoints 的差异通过 IsGateway 字段标记。
func buildRouteEntries(pluginName string, manifest *pluginsdk.ManifestResponse, httpAddr string) []RouteEntry {
	if manifest == nil || httpAddr == "" {
		return nil
	}
	proxyURL := normalizeProxyURL(httpAddr)

	entries := make([]RouteEntry, 0, len(manifest.GatewayEndpoints)+len(manifest.PluginEndpoints))
	entries = append(entries, expandEndpoints(pluginName, manifest.GatewayEndpoints, proxyURL, true)...)
	entries = append(entries, expandEndpoints(pluginName, manifest.PluginEndpoints, proxyURL, false)...)
	return entries
}

func expandEndpoints(pluginName string, decls []*pluginsdk.EndpointDeclaration, proxyURL string, isGateway bool) []RouteEntry {
	out := make([]RouteEntry, 0, len(decls))
	for _, ep := range decls {
		if ep == nil || ep.GetPath() == "" {
			continue
		}
		methods := ep.GetMethods()
		if len(methods) == 0 {
			methods = []string{"*"}
		}
		auth := ep.GetAuthType()
		if auth == "" {
			auth = AuthTypeNone
		}
		for _, mth := range methods {
			out = append(out, RouteEntry{
				Method:     strings.ToUpper(mth),
				PathPrefix: ep.GetPath(),
				PluginName: pluginName,
				AuthType:   auth,
				ProxyURL:   proxyURL,
				IsGateway:  isGateway,
			})
		}
	}
	return out
}

// normalizeProxyURL 把 host:port 形式补成 http://host:port。
func normalizeProxyURL(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}
