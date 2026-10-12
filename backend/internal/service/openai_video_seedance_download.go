package service

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const aliyunVideoResultHost = "*.aliyuncs.com"

func (s *OpenAIGatewayService) zhongYVideoDownloadURL(account *Account, raw string) (string, error) {
	base, err := url.Parse(account.GetOpenAIBaseURL())
	if err != nil {
		return "", fmt.Errorf("invalid video account URL")
	}
	target, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid video download URL")
	}
	target = base.ResolveReference(target)
	if target.Scheme != "https" || target.User != nil || target.Fragment != "" {
		return "", fmt.Errorf("unsupported video download URL")
	}
	// Keep the original escaped path and signed query intact after validation.
	if _, err = urlvalidator.ValidateHTTPSURL(target.String(), urlvalidator.ValidationOptions{}); err != nil {
		return "", fmt.Errorf("video download URL rejected")
	}
	if _, err = s.validateOutboundURL(target.String()); err != nil {
		if !isAliyunVideoResultHost(target.Hostname()) || !isAliyunVideoProviderAccount(account) || s.cfg == nil || !s.cfg.Security.URLAllowlist.Enabled {
			return "", fmt.Errorf("video download URL rejected by outbound policy")
		}
		// Wan3 returns signed media URLs from Aliyun OSS. Allow this provider
		// domain for video result downloads only; the shared transport still
		// enforces public DNS destinations and redirect checks.
		allowedHosts := append([]string(nil), s.cfg.Security.URLAllowlist.UpstreamHosts...)
		allowedHosts = append(allowedHosts, aliyunVideoResultHost)
		if _, err = urlvalidator.ValidateHTTPSURL(target.String(), urlvalidator.ValidationOptions{
			AllowedHosts:           allowedHosts,
			RequireAllowlist:       true,
			AllowPrivate:           s.cfg.Security.URLAllowlist.AllowPrivateHosts,
			AllowInsecureHTTPHosts: s.cfg.Security.URLAllowlist.AllowInsecureHTTPHosts,
		}); err != nil {
			return "", fmt.Errorf("video download URL rejected by outbound policy")
		}
	}
	return target.String(), nil
}

func isAliyunVideoProviderAccount(account *Account) bool {
	if account == nil {
		return false
	}
	base, err := url.Parse(account.GetOpenAIBaseURL())
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(base.Hostname()))
	return host != "" && host != "aliyuncs.com" && strings.HasSuffix(host, ".aliyuncs.com")
}

func isAliyunVideoResultHost(host string) bool {
	_, err := urlvalidator.ValidateHTTPSURL("https://"+host, urlvalidator.ValidationOptions{
		AllowedHosts:     []string{aliyunVideoResultHost},
		RequireAllowlist: true,
		AllowPrivate:     false,
	})
	return err == nil
}

// These are public/signed result links, not API calls. Never attach account
// credentials or header overrides. Validate every redirect and DNS destination.
func (s *OpenAIGatewayService) doZhongYVideoDownload(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	ctx := WithHTTPUpstreamPublicHostsOnly(WithHTTPUpstreamRedirectsDisabled(req.Context()))
	current := req.URL.String()
	for hop := 0; hop < 6; hop++ {
		target, err := s.zhongYVideoDownloadURL(account, current)
		if err != nil {
			return nil, err
		}
		next, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		next.Header.Set("Accept", "*/*")
		if value := req.Header.Get("Range"); value != "" {
			next.Header.Set("Range", value)
		}
		resp, err := s.httpUpstream.Do(next, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return resp, nil
		}
		location := strings.TrimSpace(resp.Header.Get("Location"))
		_ = resp.Body.Close()
		if location == "" {
			return nil, fmt.Errorf("video download redirect missing location")
		}
		ref, err := url.Parse(location)
		if err != nil {
			return nil, fmt.Errorf("invalid video download redirect")
		}
		current = next.URL.ResolveReference(ref).String()
	}
	return nil, fmt.Errorf("too many video download redirects")
}
