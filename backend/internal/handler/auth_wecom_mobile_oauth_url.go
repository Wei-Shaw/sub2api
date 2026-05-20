package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func buildWeComMobileAuthorizeURL(c *gin.Context, cfg weComOAuthConfig, state string) (string, error) {
	u, err := url.Parse(weComOAuthWebviewAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("appid", cfg.corpID)
	q.Set("redirect_uri", buildWeComMobileCallbackURL(c, cfg))
	q.Set("response_type", "code")
	q.Set("scope", "snsapi_privateinfo")
	q.Set("state", state)
	q.Set("agentid", cfg.agentID)
	u.RawQuery = q.Encode()
	u.Fragment = "wechat_redirect"
	return u.String(), nil
}

func buildWeComMobileCallbackURL(c *gin.Context, cfg weComOAuthConfig) string {
	if parsed, err := url.Parse(cfg.redirectURI); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Path = weComMobileOAuthCallbackPath
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	scheme := "http"
	if isRequestHTTPS(c) {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + weComMobileOAuthCallbackPath
}

func (h *AuthHandler) renderWeComMobileCallbackPage(c *gin.Context, ok bool, message string) {
	title := "企业微信授权"
	body := "授权已完成，请回到电脑页面继续。"
	if !ok {
		body = "授权失败：" + strings.TrimSpace(message)
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title></head><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;padding:32px;line-height:1.6;"><h2>%s</h2><p>%s</p></body></html>`, title, title, body))
}
