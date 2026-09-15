// auth.go 实现 Devin CLI 的 PKCE 登录链与 GetUserStatus 账号信息解码。
//
// 流程按 devin-connect auth.ts（自 chisel 3000.10.21 二进制逆向）：
//
//	URL  https://app.devin.ai/auth/cli/continue
//	       ?state=<uuid>&prompt=select_account
//	       &code_challenge=<b64url(sha256(verifier))>&code_challenge_method=S256
//	       &cli_pkce_marker=1
//	  （无 redirect_uri → 页面显示 code 供手动粘贴）
//
//	ExchangePKCEAuthorizationCode → api_key（+api_server_url/hosts）
//	ExchangeDevinCLIPKCECode      → session_token（兜底）
//
// 注意：不要用 POST api.devin.ai/auth/cli/token——它返回的是
// server.codeium.com 拒收为 "invalid api key" 的 web-session JWT。
package devin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

const (
	// WebAppURL 是 Devin 前端（登录页）地址。
	WebAppURL = "https://app.devin.ai"
	// APIURL 是 Devin 业务 API 地址（account 信息展示等）。
	APIURL = "https://api.devin.ai"
)

// PKCEPair 是一次 PKCE 登录会话的 verifier/challenge/state。
type PKCEPair struct {
	Verifier  string
	Challenge string
	State     string
}

func base64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// GeneratePKCE 生成一次 PKCE 会话参数。
func GeneratePKCE() PKCEPair {
	var verifier [32]byte
	_, _ = rand.Read(verifier[:])
	v := base64url(verifier[:])
	sum := sha256.Sum256([]byte(v))
	return PKCEPair{
		Verifier:  v,
		Challenge: base64url(sum[:]),
		State:     UUID(),
	}
}

// BuildAuthURL 构造手动粘贴形态的登录 URL（无 redirect_uri，
// 页面渲染授权码供用户复制）。
func BuildAuthURL(challenge, state string) string {
	return WebAppURL + "/auth/cli/continue" +
		"?state=" + state +
		"&prompt=select_account" +
		"&code_challenge=" + challenge +
		"&code_challenge_method=S256" +
		"&cli_pkce_marker=1"
}

var hexKeyPattern = regexp.MustCompile(`^[0-9a-f]{40,}$`)

// LooksLikeAPIKey 判断粘贴值是否已是 api key（而非授权码）。
// Connect api key = "devin-session-token$"+JWT（或 NormalizeToken
// 归一后的等价形态）；短授权码必须走 Exchange* RPC。
func LooksLikeAPIKey(value string) bool {
	if strings.HasPrefix(value, "devin-session-token$") {
		return true
	}
	if strings.HasPrefix(value, "eyJ") && len(value) > 100 {
		return true // session JWT
	}
	return hexKeyPattern.MatchString(value)
}

// MarshalExchangePKCE 构造 ExchangePKCEAuthorizationCode /
// ExchangeDevinCLIPKCECode 请求体：{1: code, 2: code_verifier}。
func MarshalExchangePKCE(code, verifier string) []byte {
	var buf []byte
	buf = appendString(buf, 1, code)
	buf = appendString(buf, 2, verifier)
	return buf
}

// ExchangedCredentials 是授权码交换的产物。
type ExchangedCredentials struct {
	// Token 是 Connect api key（devin-session-token$…）。
	Token string
	// APIServerURL 是上游返回的 api_server_url（空则回落 DefaultBaseURL）。
	APIServerURL string
	// WebappHost / APIURL / Name 是可选的账号元信息。
	WebappHost string
	APIURL     string
	Name       string
}

// protoStrings 把消息里所有 string 字段按字段号收成 map。
func protoStrings(buf []byte) map[int]string {
	out := make(map[int]string)
	iterFields(buf, func(f protoField) bool {
		if s := f.string(); s != "" {
			out[f.num] = s
		}
		return true
	})
	return out
}

// DecodeExchangeResponse 解析 ExchangePKCEAuthorizationCode 响应：
// {1: api_key, 2: name, 3: api_server_url, 4: webapp_host, 5: api_url}。
// api_key 形态不似 key 时返回错误（调用方再走 ExchangeDevinCLIPKCECode）。
func DecodeExchangeResponse(body []byte) (ExchangedCredentials, error) {
	fields := protoStrings(body)
	token := fields[1]
	if token == "" {
		return ExchangedCredentials{}, errors.New("token exchange returned no api_key")
	}
	return ExchangedCredentials{
		Token:        token,
		Name:         fields[2],
		APIServerURL: fields[3],
		WebappHost:   fields[4],
		APIURL:       fields[5],
	}, nil
}

// DecodeCLIPKCEResponse 解析 ExchangeDevinCLIPKCECode 响应：
// {1: session_token, 2: webapp_host, 3: api_url}。JWT session_token
// 由 NormalizeToken 归一为 devin-session-token$<jwt> 形态。
func DecodeCLIPKCEResponse(body []byte) (ExchangedCredentials, error) {
	fields := protoStrings(body)
	token := fields[1]
	if token == "" || !LooksLikeAPIKey(token) {
		return ExchangedCredentials{}, errors.New("token exchange returned no api_key")
	}
	return ExchangedCredentials{
		Token:      NormalizeToken(token),
		WebappHost: fields[2],
		APIURL:     fields[3],
	}, nil
}

// MarshalGetUserStatus 构造 GetUserStatus 请求体（仅 metadata：
// api_key 在 metadata.f3，空 body 会 HTTP 400）。
func MarshalGetUserStatus(token, clientVersion, os string) []byte {
	var body []byte
	body = appendMessage(body, 1, BuildMetadata(token, clientVersion, os, false))
	return body
}

// UserStatus 是 GetUserStatus 解码出的账号/计划摘要。
type UserStatus struct {
	Name               string
	Email              string
	TeamID             string
	OrgID              string
	PlanName           string
	AccountDisplayName string

	Pro          bool
	IsEnterprise bool
	CanUseCLI    bool

	// 月度配额与用量（-1 = unlimited 哨兵）。
	MonthlyPromptCredits   int64
	MonthlyFlowCredits     int64
	UsedPromptCredits      int64
	UsedFlowCredits        int64
	UsedFlexCredits        int64
	AvailablePromptCredits int64
	AvailableFlowCredits   int64
	AvailableFlexCredits   int64

	// 速率配额。
	DailyQuotaRemainingPercent  int64
	WeeklyQuotaRemainingPercent int64
	DailyQuotaResetAtUnix       int64
	WeeklyQuotaResetAtUnix      int64

	// ACU 用量（compute unit 计费）。
	ACUConsumed float64
	ACULimit    float64
}

// DecodeUserStatus 解析 GetUserStatus 响应：
// {1: user_status{1:pro,3:name,5:team_id,7:email,13:plan_status{...},
// 28:used_prompt,29:used_flow}, 2: plan_info{2:plan_name,12:monthly_prompt,
// 13:monthly_flow,16:is_enterprise,33:devin_info{2:can_use_cli,4:org_id,
// 8:account_display_name}}}。
func DecodeUserStatus(body []byte) *UserStatus {
	info := &UserStatus{}
	var userStatus, planInfo []byte
	iterFields(body, func(f protoField) bool {
		switch f.num {
		case 1:
			userStatus = f.bytes()
		case 2:
			planInfo = f.bytes()
		}
		return true
	})
	if userStatus != nil {
		var planStatus []byte
		iterFields(userStatus, func(f protoField) bool {
			switch f.num {
			case 1:
				info.Pro = f.bool()
			case 3:
				info.Name = f.string()
			case 5:
				info.TeamID = f.string()
			case 7:
				info.Email = f.string()
			case 13:
				planStatus = f.bytes()
			case 28:
				info.UsedPromptCredits, _ = f.signedInt()
			case 29:
				info.UsedFlowCredits, _ = f.signedInt()
			}
			return true
		})
		if planStatus != nil {
			iterFields(planStatus, func(f protoField) bool {
				switch f.num {
				case 4:
					info.AvailableFlexCredits, _ = f.signedInt()
				case 5:
					info.UsedFlowCredits, _ = f.signedInt()
				case 6:
					info.UsedPromptCredits, _ = f.signedInt()
				case 7:
					info.UsedFlexCredits, _ = f.signedInt()
				case 8:
					info.AvailablePromptCredits, _ = f.signedInt()
				case 9:
					info.AvailableFlowCredits, _ = f.signedInt()
				case 14:
					info.DailyQuotaRemainingPercent = f.int()
				case 15:
					info.WeeklyQuotaRemainingPercent = f.int()
				case 17:
					info.DailyQuotaResetAtUnix = f.int()
				case 18:
					info.WeeklyQuotaResetAtUnix = f.int()
				case 19:
					info.ACUConsumed, _ = f.double()
				case 20:
					info.ACULimit, _ = f.double()
				}
				return true
			})
		}
	}
	if planInfo != nil {
		var devinInfo []byte
		iterFields(planInfo, func(f protoField) bool {
			switch f.num {
			case 2:
				info.PlanName = f.string()
			case 12:
				info.MonthlyPromptCredits, _ = f.signedInt()
			case 13:
				info.MonthlyFlowCredits, _ = f.signedInt()
			case 16:
				info.IsEnterprise = f.bool()
			case 33:
				devinInfo = f.bytes()
			}
			return true
		})
		if devinInfo != nil {
			iterFields(devinInfo, func(f protoField) bool {
				switch f.num {
				case 2:
					info.CanUseCLI = f.bool()
				case 4:
					info.OrgID = f.string()
				case 8:
					info.AccountDisplayName = f.string()
				}
				return true
			})
		}
	}
	return info
}
