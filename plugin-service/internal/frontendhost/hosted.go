package frontendhost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const pluginAuthBridgeScript = `<script>
    (function () {
      function getPluginAuthToken() {
        try {
          var params = new URLSearchParams(window.location.search);
          var urlToken = params.get("token") || params.get("session");
          if (urlToken) {
            return urlToken;
          }
        } catch (error) {}

        try {
          return window.localStorage.getItem("auth_token") || "";
        } catch (error) {
          return "";
        }
      }

      function shouldAttachPluginAuth(url) {
        return /\/plugins\/[^/?#]+\/api(?:\/|$)/.test(url);
      }

      function mergeHeaders(baseHeaders, extraHeaders) {
        var headers = new Headers(baseHeaders || {});
        if (extraHeaders) {
          new Headers(extraHeaders).forEach(function (value, key) {
            headers.set(key, value);
          });
        }
        return headers;
      }

      var originalFetch = window.fetch.bind(window);
      window.fetch = function (input, init) {
        var requestUrl = typeof input === "string" ? input : (input && input.url) || "";
        if (!shouldAttachPluginAuth(requestUrl)) {
          return originalFetch(input, init);
        }

        var token = getPluginAuthToken();
        if (!token) {
          return originalFetch(input, init);
        }

        if (input instanceof Request) {
          var requestHeaders = mergeHeaders(input.headers, init && init.headers);
          if (!requestHeaders.has("Authorization")) {
            requestHeaders.set("Authorization", "Bearer " + token);
          }
          return originalFetch(new Request(input, { headers: requestHeaders }), init);
        }

        var nextInit = init ? Object.assign({}, init) : {};
        var headers = mergeHeaders(null, nextInit.headers);
        if (!headers.has("Authorization")) {
          headers.set("Authorization", "Bearer " + token);
        }
        nextInit.headers = headers;
        return originalFetch(input, nextInit);
      };
    })();
  </script>`

type HostedPluginOptions struct {
	PluginKey         string
	WebRoot           string
	AssetPrefix       string
	HTMLHeadTag       string
	FaviconResolver   func(*http.Request) string
	PageTitleResolver func(*http.Request) string
}

func RegisterHostedPlugin(mux *http.ServeMux, opts HostedPluginOptions) {
	pluginKey := strings.TrimSpace(opts.PluginKey)
	if mux == nil || pluginKey == "" {
		return
	}

	webRoot := strings.TrimSpace(opts.WebRoot)
	if webRoot == "" {
		return
	}

	assetRoot := filepath.Join(webRoot, "assets")
	indexPath := filepath.Join(webRoot, "index.html")
	pagePath := "/plugins/" + pluginKey
	trailingSlashPagePath := pagePath + "/"
	pageAssetPrefix := pagePath + "/assets/"
	assetPrefix := strings.TrimSpace(opts.AssetPrefix)
	if assetPrefix == "" {
		assetPrefix = pageAssetPrefix
	}
	if !strings.HasPrefix(assetPrefix, "/") {
		assetPrefix = "/" + assetPrefix
	}
	if !strings.HasSuffix(assetPrefix, "/") {
		assetPrefix += "/"
	}

	mux.HandleFunc("GET "+trailingSlashPagePath+"{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, pagePath, http.StatusPermanentRedirect)
	})

	mux.HandleFunc("GET "+pagePath, func(w http.ResponseWriter, r *http.Request) {
		disableFrontendCache(w)
		body, err := os.ReadFile(indexPath)
		if err != nil {
			http.Error(w, "plugin frontend not found", http.StatusNotFound)
			return
		}

		html := injectPluginAuthBridge(string(body), opts.HTMLHeadTag)
		html = strings.ReplaceAll(html, pageAssetPrefix, assetPrefix)
		html = injectPluginTitle(html, resolvePluginTitle(r, pluginKey, opts.PageTitleResolver))
		html = injectPluginFavicon(html, resolvePluginFavicon(r, opts.FaviconResolver))
		html = strings.ReplaceAll(html, assetPrefix+"app.js", assetPrefix+"app.js?v="+assetVersion(assetRoot, "app.js"))
		html = strings.ReplaceAll(html, assetPrefix+"app.css", assetPrefix+"app.css?v="+assetVersion(assetRoot, "app.css"))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	})

	assets := http.StripPrefix(assetPrefix, http.FileServer(http.Dir(assetRoot)))
	mux.Handle("GET "+assetPrefix+"{asset...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		disableFrontendCache(w)
		assets.ServeHTTP(w, r)
	}))
}

func injectPluginAuthBridge(html string, headTag string) string {
	tag := strings.TrimSpace(headTag)
	if tag == "" {
		tag = `<script type="module"`
	}
	index := strings.Index(html, tag)
	if index < 0 {
		return html
	}
	return html[:index] + pluginAuthBridgeScript + "\n  " + html[index:]
}

func injectPluginFavicon(html string, faviconURL string) string {
	favicon := strings.TrimSpace(faviconURL)
	if favicon == "" {
		favicon = "/logo.png"
	}

	iconTag := `<link rel="icon" type="image/png" href="` + htmlEscapeAttr(favicon) + `">`
	if strings.Contains(html, `rel="icon"`) {
		return replaceHeadIconLink(html, iconTag)
	}

	headIndex := strings.Index(html, "</head>")
	if headIndex < 0 {
		return html
	}
	return html[:headIndex] + "  " + iconTag + "\n" + html[headIndex:]
}

func injectPluginTitle(html string, pageTitle string) string {
	title := strings.TrimSpace(pageTitle)
	if title == "" {
		return html
	}

	titleStart := strings.Index(html, "<title>")
	titleEnd := strings.Index(html, "</title>")
	if titleStart < 0 || titleEnd < 0 || titleEnd <= titleStart {
		return html
	}

	titleEnd += len("</title>")
	replacement := "<title>" + htmlEscapeAttr(title) + "</title>"
	return html[:titleStart] + replacement + html[titleEnd:]
}

func replaceHeadIconLink(html, iconTag string) string {
	start := strings.Index(html, `rel="icon"`)
	if start < 0 {
		return html
	}
	tagStart := strings.LastIndex(html[:start], "<link")
	if tagStart < 0 {
		return html
	}
	tagEnd := strings.Index(html[start:], ">")
	if tagEnd < 0 {
		return html
	}
	tagEnd += start + 1
	return html[:tagStart] + iconTag + html[tagEnd:]
}

func resolvePluginFavicon(r *http.Request, resolver func(*http.Request) string) string {
	if resolver != nil {
		if favicon := strings.TrimSpace(resolver(r)); favicon != "" {
			return favicon
		}
	}
	settings := fetchPublicSettings(r)
	if settings == nil {
		return ""
	}
	return strings.TrimSpace(settings.SiteLogo)
}

func resolvePluginTitle(r *http.Request, pluginKey string, resolver func(*http.Request) string) string {
	if resolver != nil {
		if title := strings.TrimSpace(resolver(r)); title != "" {
			return title
		}
	}
	settings := fetchPublicSettings(r)
	if settings == nil {
		return ""
	}
	return strings.TrimSpace(resolveMenuLabelForPlugin(pluginKey, settings.CustomMenuItems))
}

type publicSettingsPayload struct {
	SiteLogo        string                   `json:"site_logo"`
	CustomMenuItems []publicSettingsMenuItem `json:"custom_menu_items"`
}

type publicSettingsMenuItem struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func fetchPublicSettings(r *http.Request) *publicSettingsPayload {
	baseURL := strings.TrimRight(strings.TrimSpace(resolveMainSiteBaseURL(r)), "/")
	if baseURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/settings/public", nil)
	if err != nil {
		return nil
	}
	copyMainSiteCredentials(req, r)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var envelope struct {
		Code int                   `json:"code"`
		Data publicSettingsPayload `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil
	}
	if envelope.Code != 0 {
		return nil
	}
	return &envelope.Data
}

func resolveMenuLabelForPlugin(pluginKey string, menuItems []publicSettingsMenuItem) string {
	for _, item := range menuItems {
		if pluginPathFromMenuURL(item.URL) == "/plugins/"+strings.TrimSpace(pluginKey) {
			return item.Label
		}
	}
	return ""
}

func pluginPathFromMenuURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if parsed.Hostname() != "plugin-server" {
		return ""
	}
	pluginPath := strings.TrimSpace(parsed.Path)
	if matched := strings.HasPrefix(pluginPath, "/plugins/"); !matched {
		return ""
	}
	segments := strings.Split(strings.Trim(pluginPath, "/"), "/")
	if len(segments) < 2 {
		return ""
	}
	return "/" + strings.Join(segments[:2], "/")
}

func resolveMainSiteBaseURL(r *http.Request) string {
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PLUGIN_MAIN_SERVICE_BASE_URL")), "/"); baseURL != "" {
		return baseURL
	}
	return resolveRequestBaseURL(r)
}

func resolveRequestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	if origin := resolveFrameAncestorOrigin(r); origin != "" {
		return origin
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		proto = "http"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func resolveFrameAncestorOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host != "" {
		if proto == "" {
			proto = "http"
		}
		return strings.TrimRight(proto+"://"+host, "/")
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		if u, err := url.Parse(referer); err == nil && u.Scheme != "" && u.Host != "" {
			return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
		}
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		if u, err := url.Parse(origin); err == nil && u.Scheme != "" && u.Host != "" {
			return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
		}
	}
	return ""
}

func copyMainSiteCredentials(dst *http.Request, src *http.Request) {
	if dst == nil || src == nil {
		return
	}
	if token := firstNonEmptyQueryValue(src, "token", "session"); token != "" {
		dst.Header.Set("Authorization", "Bearer "+token)
	}
	if authHeader := strings.TrimSpace(src.Header.Get("Authorization")); authHeader != "" && dst.Header.Get("Authorization") == "" {
		dst.Header.Set("Authorization", authHeader)
	}
	if rawCookie := strings.TrimSpace(src.Header.Get("Cookie")); rawCookie != "" {
		dst.Header.Set("Cookie", rawCookie)
	}
}

func firstNonEmptyQueryValue(r *http.Request, keys ...string) string {
	if r == nil || r.URL == nil {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func htmlEscapeAttr(raw string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`"`, "&quot;",
		`<`, "&lt;",
		`>`, "&gt;",
	)
	return replacer.Replace(raw)
}

func disableFrontendCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func assetVersion(assetRoot string, name string) string {
	info, err := os.Stat(filepath.Join(assetRoot, name))
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(info.ModTime().UnixNano(), 10)
}
