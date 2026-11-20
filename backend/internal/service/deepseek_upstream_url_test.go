//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDeepseekToolsURLUsesProtocolOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, legacy, chat, anthropic, wantBase, wantAnthropic string
	}{
		{"default", "", "", "", "", DefaultDeepseekBaseURL, DefaultDeepseekBaseURL + "/anthropic"},
		{"chat", APIProtocolChatCompletions, "https://relay.test/ds/v1/", "", "", "https://relay.test/ds", "https://relay.test/ds/anthropic"},
		{"anthropic", APIProtocolAnthropic, "https://relay.test/ds/anthropic/v1", "", "", "https://relay.test/ds", "https://relay.test/ds/anthropic"},
		{"custom anthropic root", APIProtocolAnthropic, "https://relay.test/custom", "", "", "https://relay.test/custom", "https://relay.test/custom"},
		{"adaptive explicit", APIProtocolAdaptive, "https://stale.test", "https://chat.test/ds/v1", "https://messages.test/native", "https://chat.test/ds", "https://messages.test/native"},
		{"adaptive chat only", APIProtocolAdaptive, "", "https://relay.test/prefix/v1/", "", "https://relay.test/prefix", "https://relay.test/prefix/anthropic"},
		{"adaptive legacy only", APIProtocolAdaptive, "https://relay.test", "", "", "https://relay.test", "https://relay.test/anthropic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := newDeepseekToolsTestAccount(tc.legacy)
			account.Credentials["api_protocol"] = tc.protocol
			account.Credentials["api_base_urls"] = map[string]any{APIProtocolChatCompletions: tc.chat, APIProtocolAnthropic: tc.anthropic}
			for _, endpoint := range []string{"search", "fetch", "ocr", "asr", "tts", "files_create", "files_get", "files_delete"} {
				path := "/v1/tools/" + endpoint
				switch endpoint {
				case "files_create":
					path = "/v1/files"
				case "files_get", "files_delete":
					path = "/v1/files/file-abc_1"
				}
				svc := &OpenAIGatewayService{}
				u, err := buildDeepseekToolsURL(account, endpoint, "file-abc_1", false, svc.validateUpstreamBaseURL)
				require.NoError(t, err)
				require.Equal(t, tc.wantBase+path, u.String())
				if endpoint == "files_create" || endpoint == "files_get" || endpoint == "files_delete" {
					u, err = buildDeepseekToolsURL(account, endpoint, "file-abc_1", true, svc.validateUpstreamBaseURL)
					require.NoError(t, err)
					require.Equal(t, tc.wantAnthropic+path, u.String())
				}
			}
		})
	}
}

func TestDeepseekToolsURLValidation(t *testing.T) {
	account := newDeepseekToolsTestAccount("https://relay.test")
	svc := &OpenAIGatewayService{}
	for _, id := range []string{"", "file/../etc", "file id", "file%20id", "file/other"} {
		for _, anthropic := range []bool{false, true} {
			_, err := buildDeepseekToolsURL(account, "files_delete", id, anthropic, svc.validateUpstreamBaseURL)
			require.ErrorContains(t, err, "invalid file id")
		}
	}
	for _, endpoint := range []string{"unknown", "files_list"} {
		_, err := buildDeepseekToolsURL(account, endpoint, "", false, svc.validateUpstreamBaseURL)
		require.ErrorContains(t, err, "unsupported")
	}
	svc.cfg = &config.Config{}
	svc.cfg.Security.URLAllowlist.Enabled = true
	svc.cfg.Security.URLAllowlist.UpstreamHosts = []string{"allowed.test"}
	for _, anthropic := range []bool{false, true} {
		_, err := buildDeepseekToolsURL(account, "files_create", "", anthropic, svc.validateUpstreamBaseURL)
		require.ErrorContains(t, err, "host is not allowed")
	}
}
