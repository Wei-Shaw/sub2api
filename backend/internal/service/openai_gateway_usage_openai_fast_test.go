package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupBillsOpenAITierAtStandard(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	for _, tt := range []struct {
		name          string
		platform      string
		freeFast      bool
		freeUltrafast bool
		tier          string
		want          bool
	}{
		{name: "free fast bills priority", platform: PlatformOpenAI, freeFast: true, tier: OpenAIFastTierPriority, want: true},
		{name: "free fast accepts fast alias", platform: PlatformOpenAI, freeFast: true, tier: "fast", want: true},
		{name: "free fast leaves ultrafast charged", platform: PlatformOpenAI, freeFast: true, tier: OpenAIFastTierUltrafast},
		{name: "free ultrafast bills ultrafast", platform: PlatformOpenAI, freeUltrafast: true, tier: OpenAIFastTierUltrafast, want: true},
		{name: "free ultrafast leaves fast charged", platform: PlatformOpenAI, freeUltrafast: true, tier: OpenAIFastTierPriority},
		{name: "both flags cover both tiers", platform: PlatformOpenAI, freeFast: true, freeUltrafast: true, tier: OpenAIFastTierUltrafast, want: true},
		{name: "standard is never rewritten", platform: PlatformOpenAI, freeFast: true, freeUltrafast: true, tier: OpenAIFastTierDefault},
		{name: "composite group is supported", platform: PlatformComposite, freeFast: true, tier: OpenAIFastTierPriority, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &APIKey{Group: &Group{
				Platform:            tt.platform,
				FreeOpenAIFast:      tt.freeFast,
				FreeOpenAIUltrafast: tt.freeUltrafast,
			}}
			require.Equal(t, tt.want, groupBillsOpenAITierAtStandard(apiKey, account, tt.tier))
		})
	}
	require.False(t, groupBillsOpenAITierAtStandard(&APIKey{Group: &Group{Platform: PlatformOpenAI, FreeOpenAIFast: true}}, &Account{Platform: PlatformAnthropic}, OpenAIFastTierPriority))
}
