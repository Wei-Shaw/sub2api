//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 上游协议分流矩阵：经真实入口（Forward / ForwardAsChatCompletions /
// ForwardAsAnthropic）遍历 平台 × api_protocol × 探针 × 账号类型 × 接入模式 ×
// protocol_rules × 入站 × 模型，从录制到的上游 URL 与请求体推断实际走的协议
// 与是否做了 Responses 形状转换，并与 legacyUpstreamRouting（统一判定前三个
// 入口各自的分流顺序）逐条比对。
//
// 设置 ROUTING_MATRIX_DUMP=<path> 时把全部观测（含 URL、上游模型、错误）按行
// 写入文件，便于在两个版本间直接 diff。

type routingMatrixIngress struct {
	name          string
	path          string
	responsesBody bool
	forward       func(*OpenAIGatewayService, *gin.Context, *Account, []byte) error
}

func routingMatrixIngresses() []routingMatrixIngress {
	return []routingMatrixIngress{
		{
			name: "responses", path: "/v1/responses", responsesBody: true,
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.Forward(context.Background(), c, account, body)
				return err
			},
		},
		{
			name: "chat", path: "/v1/chat/completions",
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				return err
			},
		},
		{
			name: "chat_responses_shape", path: "/v1/chat/completions", responsesBody: true,
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				return err
			},
		},
		{
			name: "messages", path: "/v1/messages",
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
				return err
			},
		},
	}
}

type routingMatrixCase struct {
	platform    string
	apiProtocol string
	mode        string
	accountType string
	probe       string
	rules       bool
	ingress     routingMatrixIngress
	model       string
}

func (tc routingMatrixCase) key() string {
	return fmt.Sprintf("%s|proto=%s|mode=%s|type=%s|probe=%s|rules=%t|%s|%s",
		tc.platform, tc.apiProtocol, tc.mode, tc.accountType, tc.probe, tc.rules, tc.ingress.name, tc.model)
}

func (tc routingMatrixCase) account() *Account {
	credentials := map[string]any{
		"api_key":  "sk-test",
		"base_url": "http://base.example",
		"api_base_urls": map[string]any{
			APIProtocolChatCompletions: "http://cc.example",
			APIProtocolResponses:       "http://responses.example",
			APIProtocolAnthropic:       "http://anthropic.example",
		},
	}
	if tc.apiProtocol != "" {
		credentials["api_protocol"] = tc.apiProtocol
	}
	if tc.mode != "" {
		credentials["account_mode"] = tc.mode
	}
	if tc.rules {
		credentials["protocol_rules"] = []any{
			map[string]any{"pattern": "glm-*", "protocol": APIProtocolAnthropic},
			map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses},
		}
	}
	extra := map[string]any{}
	switch tc.probe {
	case "yes":
		extra[openai_compat.ExtraKeyResponsesSupported] = true
	case "no":
		extra[openai_compat.ExtraKeyResponsesSupported] = false
	}
	return &Account{
		ID:          801,
		Name:        "routing-matrix",
		Platform:    tc.platform,
		Type:        tc.accountType,
		Concurrency: 1,
		Credentials: credentials,
		Extra:       extra,
	}
}

func (tc routingMatrixCase) body() []byte {
	switch {
	case tc.ingress.responsesBody:
		return []byte(fmt.Sprintf(`{"model":%q,"input":"hello","max_output_tokens":32,"stream":false}`, tc.model))
	case tc.ingress.name == "messages":
		return []byte(fmt.Sprintf(`{"model":%q,"max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`, tc.model))
	default:
		return []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"stream":false}`, tc.model))
	}
}

func routingMatrixCases() []routingMatrixCase {
	var cases []routingMatrixCase
	protocols := []string{"", APIProtocolAdaptive, APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses}
	types := []string{AccountTypeAPIKey, AccountTypeUpstream, AccountTypeOAuth}
	probes := []string{"", "yes", "no"}
	add := func(platform string, modes, models []string, rulesOptions []bool, probeOptions, typeOptions []string, ingresses []routingMatrixIngress) {
		for _, proto := range protocols {
			for _, mode := range modes {
				for _, accountType := range typeOptions {
					for _, probe := range probeOptions {
						for _, rules := range rulesOptions {
							for _, ingress := range ingresses {
								for _, model := range models {
									cases = append(cases, routingMatrixCase{
										platform: platform, apiProtocol: proto, mode: mode, accountType: accountType,
										probe: probe, rules: rules, ingress: ingress, model: model,
									})
								}
							}
						}
					}
				}
			}
		}
	}
	all := routingMatrixIngresses()
	add(PlatformKimi, []string{AccountModePayG}, []string{"kimi-k2"}, []bool{false}, probes, types, all)
	add(PlatformZhipu, []string{AccountModePayG}, []string{"glm-4.7"}, []bool{false}, probes, types, all)
	add(PlatformDeepseek, []string{AccountModePayG}, []string{"deepseek-chat"}, []bool{false}, probes, types, all)
	add(PlatformOpenAI, []string{""}, []string{"gpt-5"}, []bool{false}, probes, types, all)
	add(PlatformOpenCodeGo, []string{AccountModeGo, AccountModeZen},
		[]string{"gpt-5.6-luna", "minimax-m3", "glm-5.3", "claude-sonnet-4"},
		[]bool{false, true}, []string{"", "no"}, []string{AccountTypeAPIKey, AccountTypeUpstream}, all)
	// Grok 仅 /v1/messages 经统一判定；另两个入口在判定前走 Grok 专属链路。
	add(PlatformGrok, []string{""}, []string{"grok-4"}, []bool{false}, probes, types, all[3:])
	return cases
}

type routingObservation struct {
	urls      []string
	lastBody  []byte
	errText   string
	panicText string
}

func (o routingObservation) protocol() string {
	if len(o.urls) == 0 {
		return ""
	}
	last := o.urls[len(o.urls)-1]
	switch {
	case strings.HasSuffix(last, "/chat/completions"):
		return APIProtocolChatCompletions
	case strings.HasSuffix(last, "/messages"):
		return APIProtocolAnthropic
	case strings.HasSuffix(last, "/responses"):
		return APIProtocolResponses
	default:
		return "other:" + last
	}
}

// bodyKind 报告最后一个上游请求体的形状：Chat Completions 上游收到 messages
// 说明 Responses 形状请求体已被转换，收到 input 说明原样透传。
func (o routingObservation) bodyKind() string {
	switch {
	case gjson.GetBytes(o.lastBody, "messages").Exists():
		return "messages"
	case gjson.GetBytes(o.lastBody, "input").Exists():
		return "input"
	default:
		return "other"
	}
}

func (o routingObservation) String() string {
	if o.panicText != "" {
		return "panic: " + o.panicText
	}
	if len(o.urls) == 0 {
		return "no-upstream: " + o.errText
	}
	return fmt.Sprintf("urls=%s body=%s model=%s",
		strings.Join(o.urls, ","), o.bodyKind(), gjson.GetBytes(o.lastBody, "model").String())
}

func observeRouting(tc routingMatrixCase) (obs routingObservation) {
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	body := tc.body()
	defer func() {
		if r := recover(); r != nil {
			obs.panicText = fmt.Sprint(r)
		}
		for _, req := range upstream.requests {
			obs.urls = append(obs.urls, req.URL.String())
		}
		obs.lastBody = upstream.lastBody
	}()
	err := tc.ingress.forward(svc, adaptiveProtocolTestContext(tc.ingress.path, body), tc.account(), body)
	if err != nil {
		obs.errText = err.Error()
	}
	return obs
}

// legacyOpenCodeProtocol 复述统一判定前 OpenCode 的分流（原 openCodeGoNativeProtocol）：
// 显式协议 → 账号 protocol_rules（已配置但未命中为 Chat Completions）→ 接入模式的内置规则表。
func legacyOpenCodeProtocol(account *Account, model string) string {
	switch proto := account.GetAPIProtocol(); proto {
	case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return proto
	}
	if rules, configured := account.configuredProtocolRules(); configured {
		return matchProtocolRules(model, rules)
	}
	if account.IsOpenCodeZen() {
		return matchProtocolRules(model, DefaultOpenCodeZenProtocolRules())
	}
	return matchProtocolRules(model, DefaultOpenCodeGoProtocolRules())
}

// legacyUpstreamRouting 复述统一判定前三个入口各自的分流顺序，返回预期的上游协议
// 以及 Chat Completions 上游是否先把 Responses 形状请求体转换成 messages。
func legacyUpstreamRouting(account *Account, ingress routingMatrixIngress, model string) (string, bool) {
	openCode := account.IsOpenCodeGo()
	var openCodeProto string
	if openCode {
		openCodeProto = legacyOpenCodeProtocol(account, model)
	}
	switch ingress.name {
	case "responses":
		if openCode && openCodeProto != APIProtocolResponses {
			return openCodeProto, false
		}
		if account.IsAnthropicProtocol() {
			return APIProtocolAnthropic, false
		}
		// 有意差异：adaptive 且供应商无原生 Responses 端点时，统一判定对任何账号类型
		// 都回落 Chat Completions（旧 /v1/responses 入口对非 API Key 账号会误发到
		// 不存在的 /responses；Chat Completions 入口本就如此处理）。
		if account.IsAdaptiveAPIProtocol() && !account.SupportsNativeCNResponses() {
			return APIProtocolChatCompletions, false
		}
		if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
			return APIProtocolChatCompletions, false
		}
		return APIProtocolResponses, false
	case "chat", "chat_responses_shape":
		shape := ingress.responsesBody
		if openCode && openCodeProto != APIProtocolResponses {
			return openCodeProto, shape
		}
		if account.IsAdaptiveAPIProtocol() && !openCode {
			if !shape {
				return APIProtocolChatCompletions, false
			}
			if !account.SupportsNativeCNResponses() {
				return APIProtocolChatCompletions, true
			}
		}
		if account.IsAnthropicProtocol() {
			return APIProtocolAnthropic, false
		}
		if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
			return APIProtocolChatCompletions, false
		}
		return APIProtocolResponses, false
	case "messages":
		if openCode {
			if openCodeProto != APIProtocolResponses {
				return openCodeProto, false
			}
		} else if account.IsAnthropicProtocol() || account.IsAdaptiveAPIProtocol() {
			return APIProtocolAnthropic, false
		}
		if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
			return APIProtocolChatCompletions, false
		}
		return APIProtocolResponses, false
	}
	panic("unknown ingress " + ingress.name)
}

func TestUpstreamProtocolRoutingMatrixMatchesLegacy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := routingMatrixCases()
	observations := make([]routingObservation, len(cases))
	dump := make([]string, 0, len(cases))
	for i, tc := range cases {
		observations[i] = observeRouting(tc)
		dump = append(dump, tc.key()+" => "+observations[i].String())
	}
	if path := os.Getenv("ROUTING_MATRIX_DUMP"); path != "" {
		sort.Strings(dump)
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(dump, "\n")+"\n"), 0o644))
	}

	compared := 0
	for i, tc := range cases {
		obs := observations[i]
		if tc.accountType == AccountTypeAPIKey && tc.platform != PlatformGrok {
			require.NotEmpty(t, obs.urls, "API Key 账号应到达上游：%s\n%s", tc.key(), obs)
		}
		if obs.panicText != "" || len(obs.urls) == 0 {
			// 请求在到达上游前就失败（凭证/类型不适配），无法从 URL 推断协议；
			// 这类组合由 ROUTING_MATRIX_DUMP 的跨版本 diff 覆盖。
			continue
		}
		compared++
		account := tc.account()
		model := resolveMappedUpstreamModel(account, tc.body(), "")
		wantProto, wantConverted := legacyUpstreamRouting(account, tc.ingress, model)
		require.Equal(t, wantProto, obs.protocol(), "%s\n%s", tc.key(), obs)
		if wantProto == APIProtocolChatCompletions && tc.ingress.name == "chat_responses_shape" {
			wantKind := "input"
			if wantConverted {
				wantKind = "messages"
			}
			require.Equal(t, wantKind, obs.bodyKind(), "%s\n%s", tc.key(), obs)
		}
	}
	require.Positive(t, compared)
}
