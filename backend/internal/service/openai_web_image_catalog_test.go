package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAvailableModelsFreeWebImagesIgnoreChatMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    any
		consent bool
		plan    string
		want    bool
	}{
		{"enabled", true, true, "free", true},
		{"disabled", false, true, "free", false},
		{"unconfigured", nil, true, "free", false},
		{"no history consent", true, false, "free", false},
		{"paid account", true, true, "plus", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(3)
			account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: map[string]any{"plan_type": tc.plan,
					"model_mapping": map[string]any{"only-chat": "gpt-6-luna"}},
				Extra: map[string]any{AccountExtraChatGPTWebImageRegularChat: tc.consent}}
			if tc.flag != nil {
				account.Extra[AccountExtraChatGPTWebImageEnabled] = tc.flag
			}
			svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{
				byGroup: map[int64][]Account{groupID: {account}}, all: []Account{account}}}
			for _, platform := range []string{PlatformOpenAI, ""} {
				models := svc.GetAvailableModels(context.Background(), &groupID, platform)
				require.Contains(t, models, "only-chat")
				require.NotContains(t, models, "gpt-6-luna", "image opt-in must not widen chat access")
				for _, image := range []string{"gpt-image-1", "gpt-image-1.5", "gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
					if tc.want {
						require.Contains(t, models, image)
					} else {
						require.NotContains(t, models, image)
					}
				}
			}
		})
	}
}
