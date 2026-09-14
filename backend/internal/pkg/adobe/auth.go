package adobe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 鉴权相关请求的超时。这些调用都很轻，不需要生成链路那么长的耐心。
const (
	refreshTimeout = 30 * time.Second
	profileTimeout = 15 * time.Second
	creditsTimeout = 20 * time.Second
)

// AccountInfo 是 Adobe 账号的基本信息。
type AccountInfo struct {
	DisplayName string
	Email       string
	UserID      string
}

// RefreshResult 是一次 cookie 换 token 的结果。
type RefreshResult struct {
	AccessToken string
	// ExpiresIn 是上游给出的有效期（秒）；上游未返回时为 0，
	// 此时可用 DecodeJWTExp 从 token 自身取过期时间。
	ExpiresIn int64
	// Account 在 SkipAccountFetch 为真或查询失败时为 nil——账号信息只是附带产物，
	// 拿不到不应让整个刷新失败。
	Account *AccountInfo
	// Raw 是上游原始响应，供上层保留额外字段。
	Raw map[string]any
}

// RefreshOptions 调整刷新行为。零值表示「刷新并顺便查账号信息」。
type RefreshOptions struct {
	// SkipAccountFetch 为真时不查账号信息，省一次往返。
	SkipAccountFetch bool
}

// NormalizeCookieString 把多种 cookie 写法归一成 "k=v; k=v" 串。
//
// 接受的形态：裸字符串（可带 "Cookie:" 前缀）、字符串数组、{name,value} 对象数组，
// 以及带 cookie / cookies 键的对象。凭据是用户从浏览器导出的，形态不可控，
// 这里全部兜住而不是让上层各写一份解析。
func NormalizeCookieString(input any) string {
	switch value := input.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(value)
		// 直接从浏览器 DevTools 复制时会带上 "Cookie:" 头名。
		if strings.HasPrefix(strings.ToLower(text), "cookie:") {
			_, after, _ := strings.Cut(text, ":")
			return strings.TrimSpace(after)
		}
		return text
	case []string:
		pairs := make([]string, 0, len(value))
		for _, item := range value {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				pairs = append(pairs, trimmed)
			}
		}
		return strings.Join(pairs, "; ")
	case []any:
		return normalizeCookieList(value)
	case map[string]any:
		if cookies, ok := value["cookies"]; ok {
			return NormalizeCookieString(cookies)
		}
		if cookie, ok := value["cookie"]; ok {
			return NormalizeCookieString(cookie)
		}
		return ""
	default:
		return ""
	}
}

func normalizeCookieList(items []any) string {
	pairs := make([]string, 0, len(items))
	for _, item := range items {
		switch entry := item.(type) {
		case string:
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				pairs = append(pairs, trimmed)
			}
		case map[string]any:
			name, _ := entry["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			value, _ := entry["value"].(string)
			pairs = append(pairs, name+"="+strings.TrimSpace(value))
		}
	}
	return strings.Join(pairs, "; ")
}

// RefreshAccessTokenFromCookie 用 cookie 换一个短期 IMS access_token。
//
// cookieInput 会先经 NormalizeCookieString 归一，故可直接传凭据里的原始值。
func (c *Client) RefreshAccessTokenFromCookie(
	ctx context.Context, cookieInput any, opts RefreshOptions,
) (*RefreshResult, error) {
	cookie := NormalizeCookieString(cookieInput)
	if cookie == "" {
		return nil, NewRequestError("cookie is required")
	}

	form := url.Values{}
	form.Set("client_id", c.identity.IMSClientID)
	form.Set("scope", c.identity.IMSScope)

	headers, order := newHeaderBuilder().
		set("accept", "*/*").
		set("accept-language", "en-US,en;q=0.9").
		set("content-type", "application/x-www-form-urlencoded;charset=UTF-8").
		set("cookie", cookie).
		set("origin", c.identity.Origin).
		set("referer", c.identity.Referer).
		set("user-agent", c.identity.UserAgent).
		set("sec-fetch-dest", "empty").
		set("sec-fetch-mode", "cors").
		set("sec-fetch-site", "same-site").
		build()

	resp, err := c.transport.Do(ctx, &Request{
		Method:      http.MethodPost,
		URL:         c.identity.IMSRefreshURL,
		Headers:     headers,
		HeaderOrder: order,
		Body:        []byte(form.Encode()),
		Timeout:     refreshTimeout,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("refresh request failed: %d %s", resp.StatusCode, resp.BodyPreview())
		// cookie 失效返回 401/403，需要用户重新导出——不是可重试的临时故障。
		if isAuthStatus(resp.StatusCode) {
			return nil, NewAuthError(message, resp.StatusCode)
		}
		if IsRetryableStatus(resp.StatusCode) {
			return nil, NewUpstreamTemporaryError(message, resp.StatusCode, ErrorTypeStatus)
		}
		return nil, NewRequestError(message)
	}

	var data map[string]any
	if err := json.Unmarshal(resp.Body, &data); err != nil || data == nil {
		return nil, NewRequestError("refresh response is not valid json")
	}
	accessToken, _ := data["access_token"].(string)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		keys := make([]string, 0, len(data))
		for key := range data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		message := fmt.Sprintf("refresh response missing access_token (keys=%s)", strings.Join(keys, ","))
		if errName := firstString(data, "error", "reason", "errorCode"); errName != "" {
			message += " error=" + errName
		}
		if desc := firstString(data, "error_description", "message", "errorMessage"); desc != "" {
			message += " " + desc
		}
		// IMS 在没有 sid/ims_sid 时会 200 返回 invalid_credentials，而不是 401。
		if strings.EqualFold(firstString(data, "error", "reason", "errorCode"), "invalid_credentials") {
			if strings.Contains(strings.ToLower(message), "session cookies are empty") {
				message += "; cookie is missing IMS session (ims_sid). Export cookies for adobeid-na1.services.adobe.com, not only firefly.adobe.com"
			}
			return nil, NewAuthError(message, http.StatusUnauthorized)
		}
		return nil, NewRequestError(message)
	}
	// Firefly 前端的 IMS 请求不带 guest_allowed。带上该字段时，会话 cookie
	// 不被接受会静默发 @GuestID 访客 token，credits/出图随后 403。
	if isGuestToken(accessToken) {
		return nil, NewAuthError(
			"IMS issued a guest token; logged-in session cookies were not accepted",
			http.StatusUnauthorized,
		)
	}

	result := &RefreshResult{AccessToken: accessToken, Raw: data}
	if expiresIn, ok := claimInt(data, "expires_in"); ok {
		result.ExpiresIn = expiresIn
	}
	if !opts.SkipAccountFetch {
		// 账号信息拿不到不影响刷新本身。
		if account, err := c.FetchAccountInfo(ctx, accessToken); err == nil {
			result.Account = account
		}
	}
	return result, nil
}

// FetchAccountInfo 用 access_token 查账号信息。
//
// 依次尝试两个 IMS profile 端点：不同账号归属的 region 不同，单个端点会对部分账号 404。
func (c *Client) FetchAccountInfo(ctx context.Context, accessToken string) (*AccountInfo, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, NewRequestError("empty access token")
	}

	headers, order := c.browserHeaders().
		set("authorization", "Bearer "+token).
		set("accept", "application/json").
		build()

	for _, endpoint := range profileURLs {
		resp, err := c.transport.Do(ctx, &Request{
			Method:      http.MethodGet,
			URL:         endpoint,
			Headers:     headers,
			HeaderOrder: order,
			Timeout:     profileTimeout,
		})
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(resp.Body, &data); err != nil {
			continue
		}
		info := &AccountInfo{
			DisplayName: firstString(data, "displayName", "name", "fullName"),
			Email:       firstString(data, "email"),
			UserID:      firstString(data, "userId", "authId"),
		}
		if info.DisplayName != "" || info.Email != "" || info.UserID != "" {
			return info, nil
		}
	}
	return nil, NewRequestError("account info unavailable")
}

// CreditsBalance 是 Firefly 的额度余额。各字段在上游未返回时为 nil，
// 与「返回 0」区分开——前者是拿不到，后者是真的用完了。
type CreditsBalance struct {
	Total     *int64
	Used      *int64
	Available *int64
	// AvailableUntil 是额度有效期，上游格式不稳定，原样保留。
	AvailableUntil any

	// PlanCap 是上游的套餐标签（如 "FREE"）。不做枚举归一化——只见过 FREE 一档，
	// 付费档字面量（PAID/PRO/PREMIUM/…）无实测数据，猜就是错。空串 = 上游没返回。
	PlanCap string
	// CreditPools 是 credits 对象下每个子池的展开。池名（如 firefly_free_credit）
	// 随套餐变化，遍历而不是硬编码找特定 key。
	CreditPools []CreditPool
}

// CreditPool 是 credits.<pool_name>.quota 的解析结果。
type CreditPool struct {
	Name      string
	Total     *int64
	Used      *int64
	Available *int64
}

// FetchCreditsBalance 查询账号的 Firefly 额度余额。
func (c *Client) FetchCreditsBalance(ctx context.Context, accessToken string) (*CreditsBalance, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, NewRequestError("empty access token")
	}
	accountID := normalizeAdobeAccountID(AccountIDFromToken(token))
	if accountID == "" {
		return nil, NewRequestError("missing account id")
	}

	headers, order := c.browserHeaders().
		set("authorization", "Bearer "+token).
		set("x-api-key", c.identity.CreditsAPIKey).
		set("x-account-id", accountID).
		set("accept", "application/json").
		set("content-type", "application/json").
		build()

	resp, err := c.transport.Do(ctx, &Request{
		Method:      http.MethodGet,
		URL:         CreditsURL,
		Headers:     headers,
		HeaderOrder: order,
		Timeout:     creditsTimeout,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if isAuthStatus(resp.StatusCode) {
			return nil, authOrQuotaError(resp)
		}
		message := fmt.Sprintf("credits request failed: %d", resp.StatusCode)
		if IsRetryableStatus(resp.StatusCode) {
			return nil, NewUpstreamTemporaryError(message, resp.StatusCode, ErrorTypeStatus)
		}
		return nil, NewRequestError(message)
	}

	var payload struct {
		Total struct {
			Quota struct {
				Total     *int64 `json:"total"`
				Used      *int64 `json:"used"`
				Available *int64 `json:"available"`
			} `json:"quota"`
			AvailableUntil any    `json:"availableUntil"`
			PlanCap        string `json:"planCap"`
		} `json:"total"`
		Credits map[string]struct {
			Quota struct {
				Total     *int64 `json:"total"`
				Used      *int64 `json:"used"`
				Available *int64 `json:"available"`
			} `json:"quota"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, NewRequestError("credits response is not valid json")
	}
	// credits 里池名随套餐变化，遍历——但键名字典序落切片，防止相同请求下顺序抖动。
	var pools []CreditPool
	if len(payload.Credits) > 0 {
		names := make([]string, 0, len(payload.Credits))
		for name := range payload.Credits {
			names = append(names, name)
		}
		sort.Strings(names)
		pools = make([]CreditPool, 0, len(names))
		for _, name := range names {
			pool := payload.Credits[name]
			pools = append(pools, CreditPool{
				Name:      name,
				Total:     pool.Quota.Total,
				Used:      pool.Quota.Used,
				Available: pool.Quota.Available,
			})
		}
	}
	return &CreditsBalance{
		Total:          payload.Total.Quota.Total,
		Used:           payload.Total.Quota.Used,
		Available:      payload.Total.Quota.Available,
		AvailableUntil: payload.Total.AvailableUntil,
		PlanCap:        payload.Total.PlanCap,
		CreditPools:    pools,
	}, nil
}

// firstString 返回第一个非空的字符串字段。
func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := data[key].(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return ""
}
