package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	sharedhttp "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	GrokRiskExtraKey     = "grok_risk"
	GrokRiskMaxBatchIDs  = 200
	GrokRiskMaxSSOTokens = 200
	grokRiskCheckTimeout = 20 * time.Second
)

// GrokRiskReport is the operator-facing snapshot stored on extra.grok_risk.
// It never contains SSO cookies or access tokens.
type GrokRiskReport struct {
	Verdict       string   `json:"verdict"`
	Kind          string   `json:"kind,omitempty"`
	BotFlagSource *int64   `json:"bot_flag_source,omitempty"`
	HasBFS        bool     `json:"has_bfs"`
	BFS           any      `json:"bfs,omitempty"`
	Policy        string   `json:"policy,omitempty"`
	Risk          *float64 `json:"risk,omitempty"`
	Event         string   `json:"event,omitempty"`
	Details       string   `json:"details,omitempty"`
	Denied        bool     `json:"denied,omitempty"`
	Source        string   `json:"source,omitempty"`
	Error         string   `json:"error,omitempty"`
	Email         string   `json:"email,omitempty"`
	CheckedAt     string   `json:"checked_at"`
}

type grokSSOStateInspector func(ctx context.Context, ssoToken, proxyURL string) xai.GrokAccountState

// WithSSOStateInspector overrides the grok.com homepage probe (tests).
func (s *GrokOAuthService) WithSSOStateInspector(fn grokSSOStateInspector) *GrokOAuthService {
	if s != nil {
		s.ssoStateInspector = fn
	}
	return s
}

// InspectSSOAccountState reads grok.com registration-risk fields for one SSO cookie.
func (s *GrokOAuthService) InspectSSOAccountState(ctx context.Context, ssoToken string, proxyID *int64) xai.GrokAccountState {
	proxyURL := ""
	if s != nil {
		if resolved, err := s.proxyURL(ctx, proxyID); err == nil {
			proxyURL = resolved
		} else {
			state := xai.ParseGrokAccountState("")
			state.Error = err.Error()
			return state
		}
		if s.ssoStateInspector != nil {
			return s.ssoStateInspector(ctx, ssoToken, proxyURL)
		}
	}
	client, err := grokHomeHTTPClient(proxyURL)
	if err != nil {
		state := xai.ParseGrokAccountState("")
		state.Error = err.Error()
		return state
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, grokRiskCheckTimeout)
	defer cancel()
	return xai.InspectSSOAccountState(requestCtx, ssoToken, client)
}

func grokHomeHTTPClient(proxyURL string) (*http.Client, error) {
	return sharedhttp.GetClient(sharedhttp.Options{
		ProxyURL:              proxyURL,
		Timeout:               grokRiskCheckTimeout,
		ResponseHeaderTimeout: 15 * time.Second,
	})
}

// InspectAccountTokenRisk decodes stored Grok OAuth JWTs (no network).
func InspectAccountTokenRisk(account *Account) GrokRiskReport {
	report := GrokRiskReport{
		Verdict:   xai.GrokRiskUnknown,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "jwt",
	}
	if account == nil || !account.IsGrok() {
		report.Verdict = xai.GrokRiskError
		report.Error = "not a grok account"
		return report
	}
	access := strings.TrimSpace(account.GetCredential("access_token"))
	idToken := strings.TrimSpace(account.GetCredential("id_token"))
	refresh := strings.TrimSpace(account.GetCredential("refresh_token"))
	info := xai.InspectJWTRisk(access, idToken, refresh)
	applyJWTRiskToReport(&report, info)
	if !info.OK {
		if persisted := persistedGrokTokenRisk(account); persisted != nil {
			return *persisted
		}
		report.Verdict = xai.GrokRiskError
		report.Error = "jwt decode failed or empty token"
		return report
	}
	return report
}

func applyJWTRiskToReport(report *GrokRiskReport, info xai.JWTRiskInfo) {
	if report == nil || !info.OK {
		return
	}
	report.HasBFS = info.HasBFS
	report.BFS = info.BFS
	report.BotFlagSource = info.BotFlagSource
	if info.HasBFS || (info.HasBotFlag && info.BotFlagSource != nil && *info.BotFlagSource == 1) {
		report.Verdict = xai.GrokRiskFlagged
		report.Kind = xai.GrokRiskKindJWT
		if info.Source != "" {
			report.Source = "jwt:" + info.Source
		}
		return
	}
	report.Verdict = xai.GrokRiskClean
}

func persistedGrokTokenRisk(account *Account) *GrokRiskReport {
	if account == nil || account.Credentials == nil {
		return nil
	}
	report := GrokRiskReport{
		Verdict:   xai.GrokRiskClean,
		Source:    "record",
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	found := false
	if raw, ok := account.Credentials["bfs"]; ok && raw != nil {
		found = true
		if isTruthyCredential(raw) {
			report.HasBFS = true
			report.BFS = account.Credentials["bfs_value"]
			if report.BFS == nil {
				report.BFS = raw
			}
			report.Verdict = xai.GrokRiskFlagged
			report.Kind = xai.GrokRiskKindJWT
		}
	}
	if flag, ok := credentialInt64(account.Credentials["bot_flag_source"]); ok {
		found = true
		value := flag
		report.BotFlagSource = &value
		if flag == 1 {
			report.Verdict = xai.GrokRiskFlagged
			report.Kind = xai.GrokRiskKindJWT
		}
	}
	if !found {
		return nil
	}
	return &report
}

func MergeGrokRiskReports(live xai.GrokAccountState, jwt GrokRiskReport) GrokRiskReport {
	report := GrokRiskReport{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Email:     jwt.Email,
	}
	liveVerdict := xai.ClassifyGrokAccountState(live)
	if live.Found || live.Error != "" || live.StatusCode != 0 {
		report.BotFlagSource = live.BotFlagSource
		report.Details = live.BotFlagDetails
		report.Policy = live.Policy
		report.Risk = live.Risk
		report.Event = live.Event
		report.Denied = live.Denied
		report.Error = live.Error
		report.Source = "grok.com"
		report.Verdict = liveVerdict
		if liveVerdict == xai.GrokRiskFlagged {
			report.Kind = xai.GrokFlagKind(live.BotFlagDetails)
		}
	}
	if jwt.HasBFS || jwt.BotFlagSource != nil {
		report.HasBFS = jwt.HasBFS
		report.BFS = jwt.BFS
		if report.BotFlagSource == nil {
			report.BotFlagSource = jwt.BotFlagSource
		}
		if jwt.Verdict == xai.GrokRiskFlagged && report.Verdict != xai.GrokRiskFlagged {
			report.Verdict = xai.GrokRiskFlagged
			report.Kind = xai.GrokRiskKindJWT
		}
		if report.Source == "" {
			report.Source = jwt.Source
		} else if jwt.Source != "" && !strings.Contains(report.Source, jwt.Source) {
			report.Source = report.Source + "+" + jwt.Source
		}
	}
	if report.Verdict == "" {
		if jwt.Verdict != "" {
			report.Verdict = jwt.Verdict
			report.Kind = jwt.Kind
			report.Source = jwt.Source
			report.Error = jwt.Error
		} else {
			report.Verdict = xai.GrokRiskUnknown
		}
	}
	return report
}

func GrokRiskReportFromLiveState(state xai.GrokAccountState, email string) GrokRiskReport {
	verdict := xai.ClassifyGrokAccountState(state)
	report := GrokRiskReport{
		Verdict:       verdict,
		BotFlagSource: state.BotFlagSource,
		Policy:        state.Policy,
		Risk:          state.Risk,
		Event:         state.Event,
		Details:       state.BotFlagDetails,
		Denied:        state.Denied,
		Source:        "grok.com",
		Error:         state.Error,
		Email:         email,
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if verdict == xai.GrokRiskFlagged {
		report.Kind = xai.GrokFlagKind(state.BotFlagDetails)
	}
	return report
}

func GrokRiskEmailFromSSO(ssoToken string) string {
	claims := xai.DecodeJWTClaims(xai.ExtractSSOTokenFromLine(ssoToken))
	if claims == nil {
		return ""
	}
	return xai.JWTClaimString(claims, "email")
}

func GrokRiskSnapshotMap(report GrokRiskReport) map[string]any {
	out := map[string]any{
		"verdict":    report.Verdict,
		"has_bfs":    report.HasBFS,
		"denied":     report.Denied,
		"checked_at": report.CheckedAt,
	}
	if report.Kind != "" {
		out["kind"] = report.Kind
	}
	if report.BotFlagSource != nil {
		out["bot_flag_source"] = *report.BotFlagSource
	}
	if report.HasBFS {
		out["bfs"] = report.BFS
	}
	if report.Policy != "" {
		out["policy"] = report.Policy
	}
	if report.Risk != nil {
		out["risk"] = *report.Risk
	}
	if report.Event != "" {
		out["event"] = report.Event
	}
	if report.Details != "" {
		out["details"] = report.Details
	}
	if report.Source != "" {
		out["source"] = report.Source
	}
	if report.Error != "" {
		out["error"] = report.Error
	}
	if report.Email != "" {
		out["email"] = report.Email
	}
	return out
}

func isTruthyCredential(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(value))
		return trimmed == "true" || trimmed == "1" || trimmed == "yes"
	default:
		if n, ok := credentialInt64(raw); ok {
			return n != 0
		}
		return false
	}
}

func credentialInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case float64:
		return int64(value), true
	case float32:
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	case int32:
		return int64(value), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
