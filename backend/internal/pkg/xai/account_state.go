package xai

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	GrokHomeURL          = "https://grok.com/"
	GrokRiskClean        = "clean"
	GrokRiskFlagged      = "flagged"
	GrokRiskError        = "error"
	GrokRiskUnknown      = "unknown"
	GrokRiskKindIP       = "ip"
	GrokRiskKindAccount  = "account"
	GrokRiskKindJWT      = "jwt"
	grokHomeTimeout      = 20 * time.Second
	grokHomeMaxBodyBytes = 2 << 20
)

var (
	grokBotFlagSourceRe  = regexp.MustCompile(`botFlagSource"\s*:\s*(null|-?\d+)`)
	grokBotFlagDetailsRe = regexp.MustCompile(`botFlagDetails"\s*:\s*(?:null|"([^"]*)")`)
	ipFarmMarkers        = []string{"eapi_ip_bot_farm", "no_token_farm"}
)

// GrokAccountState is the grok.com registration-risk snapshot parsed from the
// homepage RSC payload. Distinct from JWT claims bot_flag_source / bfs.
type GrokAccountState struct {
	Found          bool
	BotFlagSource  *int64
	BotFlagDetails string
	Policy         string
	Risk           *float64
	Event          string
	Denied         bool
	StatusCode     int
	URL            string
	Error          string
}

// JWTRiskInfo is the local JWT risk signal (bfs presence and bot_flag_source).
type JWTRiskInfo struct {
	OK            bool
	HasBFS        bool
	BFS           any
	HasBotFlag    bool
	BotFlagSource *int64
	Source        string
	Sub           string
	Tier          string
}

// ExtractSSOTokenFromLine accepts a bare JWT, "sso=...", cookie header, or
// pack-format "email----password----sso" / "email----sso" line.
func ExtractSSOTokenFromLine(line string) string {
	raw := strings.TrimSpace(line)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ""
	}
	if strings.Contains(raw, "----") {
		parts := strings.Split(raw, "----")
		raw = strings.TrimSpace(parts[len(parts)-1])
	}
	return NormalizeSSOToken(raw)
}

// ParseGrokAccountState extracts botFlagSource / botFlagDetails from grok.com
// HTML or Next.js RSC text. Escaped quotes (\"botFlagSource\") are unescaped
// first so the fields can be read as ordinary JSON fragments.
func ParseGrokAccountState(pageHTML string) GrokAccountState {
	normalized := strings.ReplaceAll(pageHTML, `\"`, `"`)
	sourceMatch := grokBotFlagSourceRe.FindStringSubmatch(normalized)
	detailsMatch := grokBotFlagDetailsRe.FindStringSubmatch(normalized)

	state := GrokAccountState{Found: len(sourceMatch) > 0 || len(detailsMatch) > 0}
	if len(sourceMatch) > 1 && sourceMatch[1] != "null" {
		if parsed, err := strconv.ParseInt(sourceMatch[1], 10, 64); err == nil {
			state.BotFlagSource = &parsed
		}
	}
	if len(detailsMatch) > 1 {
		state.BotFlagDetails = detailsMatch[1]
	}

	fields := parseBotFlagDetailFields(state.BotFlagDetails)
	state.Policy = strings.ToLower(fields["policy"])
	state.Event = fields["event"]
	state.Denied = state.Policy == "deny"
	if rawRisk := fields["risk"]; rawRisk != "" {
		if parsed, err := strconv.ParseFloat(rawRisk, 64); err == nil {
			state.Risk = &parsed
		}
	}
	return state
}

func parseBotFlagDetailFields(details string) map[string]string {
	out := make(map[string]string)
	for _, item := range strings.Split(details, ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

// ClassifyGrokAccountState maps a grok.com snapshot to flagged / clean / error / unknown.
// Flagged: botFlagSource in (1, 2) or policy=deny (any event, including $registration / $login).
func ClassifyGrokAccountState(state GrokAccountState) string {
	if state.Denied || state.Policy == "deny" {
		return GrokRiskFlagged
	}
	if state.BotFlagSource != nil && (*state.BotFlagSource == 1 || *state.BotFlagSource == 2) {
		return GrokRiskFlagged
	}
	if state.Found {
		return GrokRiskClean
	}
	if strings.TrimSpace(state.Error) != "" {
		return GrokRiskError
	}
	if state.StatusCode != 0 && state.StatusCode != http.StatusOK {
		return GrokRiskError
	}
	return GrokRiskUnknown
}

// GrokFlagKind splits flagged accounts into IP-layer soft marks vs account-level.
func GrokFlagKind(details string) string {
	for _, marker := range ipFarmMarkers {
		if strings.Contains(details, marker) {
			return GrokRiskKindIP
		}
	}
	return GrokRiskKindAccount
}

// InspectJWTRisk decodes access / SSO / id / refresh tokens and reports bfs
// presence (clean tokens omit the claim) plus bot_flag_source. Prefers the
// first flagged source, otherwise the first successfully decoded token.
func InspectJWTRisk(tokens ...string) JWTRiskInfo {
	result := JWTRiskInfo{}
	for _, raw := range tokens {
		token := NormalizeSSOToken(raw)
		if token == "" || strings.Count(token, ".") < 2 {
			continue
		}
		claims := DecodeJWTClaims(token)
		if claims == nil {
			continue
		}
		info := jwtRiskFromClaims(claims)
		if !result.OK {
			result = info
			result.OK = true
		}
		if info.HasBFS || (info.HasBotFlag && info.BotFlagSource != nil && *info.BotFlagSource == 1) {
			info.OK = true
			return info
		}
	}
	return result
}

func jwtRiskFromClaims(claims map[string]any) JWTRiskInfo {
	info := JWTRiskInfo{
		Sub:  firstNonEmpty(JWTClaimString(claims, "sub"), JWTClaimString(claims, "principal_id")),
		Tier: JWTClaimString(claims, "tier"),
	}
	if raw, ok := claims["bfs"]; ok && raw != nil {
		info.HasBFS = true
		info.BFS = raw
		info.Source = "bfs"
	}
	if flag, ok := JWTClaimInt64(claims, "bot_flag_source"); ok {
		value := flag
		info.HasBotFlag = true
		info.BotFlagSource = &value
		if info.Source == "" {
			info.Source = "bot_flag_source"
		}
	}
	return info
}

// HasBFSClaim reports whether the JWT carries a bfs claim (presence is the signal).
func HasBFSClaim(token string) bool {
	return InspectJWTRisk(token).HasBFS
}

// IsRiskFlaggedToken reports media/account risk from JWT: bfs present or
// bot_flag_source == 1. bot_flag_source == 2 is the grok.com IP soft mark and
// is not treated as a media-degradation token flag here.
func IsRiskFlaggedToken(token string) bool {
	info := InspectJWTRisk(token)
	if info.HasBFS {
		return true
	}
	return info.HasBotFlag && info.BotFlagSource != nil && *info.BotFlagSource == 1
}

// InspectSSOAccountState GETs grok.com with the SSO cookie and parses the
// registration-risk fields. Network / Cloudflare failures return Error set and
// do not invent a flagged verdict.
func InspectSSOAccountState(ctx context.Context, ssoToken string, client SSODeviceHTTPClient) GrokAccountState {
	state := ParseGrokAccountState("")
	token := ExtractSSOTokenFromLine(ssoToken)
	if token == "" {
		state.Error = "sso is empty"
		return state
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = &http.Client{Timeout: grokHomeTimeout}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	seedSSOCookies(jar, token)
	seedGrokHomeCookies(jar, token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GrokHomeURL, nil)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	req.Header.Set("User-Agent", ssoDefaultUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if homeURL, parseErr := url.Parse(GrokHomeURL); parseErr == nil {
		if cookie := cookieHeaderFromJar(jar, homeURL); cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	defer resp.Body.Close()
	state.StatusCode = resp.StatusCode
	if resp.Request != nil && resp.Request.URL != nil {
		state.URL = resp.Request.URL.String()
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, grokHomeMaxBodyBytes+1))
	if err != nil {
		state.Error = err.Error()
		return state
	}
	if len(body) > grokHomeMaxBodyBytes {
		state.Error = "grok.com response too large"
		return state
	}
	if resp.StatusCode != http.StatusOK {
		suffix := ""
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			suffix = " (cloudflare or egress restriction)"
		}
		state.Error = "grok.com HTTP " + strconv.Itoa(resp.StatusCode) + suffix
		return state
	}
	parsed := ParseGrokAccountState(string(body))
	state.Found = parsed.Found
	state.BotFlagSource = parsed.BotFlagSource
	state.BotFlagDetails = parsed.BotFlagDetails
	state.Policy = parsed.Policy
	state.Risk = parsed.Risk
	state.Event = parsed.Event
	state.Denied = parsed.Denied
	if !parsed.Found {
		state.Error = "grok.com botFlag fields not found"
	}
	return state
}

func seedGrokHomeCookies(jar http.CookieJar, token string) {
	if jar == nil {
		return
	}
	for _, rawURL := range []string{GrokHomeURL, "https://www.grok.com/"} {
		target, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		jar.SetCookies(target, []*http.Cookie{
			{Name: "sso", Value: token, Path: "/", Secure: true, HttpOnly: true},
			{Name: "sso-rw", Value: token, Path: "/", Secure: true, HttpOnly: true},
		})
	}
}

func cookieHeaderFromJar(jar http.CookieJar, target *url.URL) string {
	if jar == nil || target == nil {
		return ""
	}
	cookies := jar.Cookies(target)
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}
