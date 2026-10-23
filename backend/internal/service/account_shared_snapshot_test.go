//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 共享母本的副本命中预解析缓存时不计算签名：此处故意原地改写共享的原始映射，
// 只用来观察副本走的是免签名路径（生产代码不会原地修改该 map）。
func TestAccountPrepareSharedSnapshot_CopiesReusePreparedMappingWithoutSignature(t *testing.T) {
	rawMapping := map[string]any{"claude-sonnet": "sonnet-a"}
	master := &Account{
		Platform:    PlatformAnthropic,
		Credentials: map[string]any{"model_mapping": rawMapping},
	}
	master.PrepareSharedSnapshot()

	replica := *master
	require.Equal(t, map[string]string{"claude-sonnet": "sonnet-a"}, replica.GetModelMapping())

	rawMapping["claude-sonnet"] = "sonnet-b"
	require.Equal(t, "sonnet-a", replica.GetModelMapping()["claude-sonnet"],
		"the shared fast path must not recompute the signature")
	require.Equal(t, "sonnet-a", replica.GetMappedModel("claude-sonnet"))

	// 未标记为共享的账号仍按签名检出原地改写。
	plainRaw := map[string]any{"claude-sonnet": "sonnet-a"}
	plain := &Account{
		Platform:    PlatformAnthropic,
		Credentials: map[string]any{"model_mapping": plainRaw},
	}
	require.Equal(t, "sonnet-a", plain.GetModelMapping()["claude-sonnet"])
	plainRaw["claude-sonnet"] = "sonnet-b"
	require.Equal(t, "sonnet-b", plain.GetModelMapping()["claude-sonnet"])
}

func TestAccountPrepareSharedSnapshot_ReplacedCredentialsFallBackToSignaturePath(t *testing.T) {
	master := &Account{
		Platform:    PlatformAnthropic,
		Credentials: map[string]any{"model_mapping": map[string]any{"claude-sonnet": "sonnet-a"}},
	}
	master.PrepareSharedSnapshot()

	replica := *master
	replacedRaw := map[string]any{"claude-sonnet": "sonnet-b"}
	replica.Credentials = map[string]any{"model_mapping": replacedRaw}
	require.Equal(t, "sonnet-b", replica.GetModelMapping()["claude-sonnet"])

	// 重建后副本回到普通路径：同一 map 的原地改写由签名检出。
	replacedRaw["claude-sonnet"] = "sonnet-c"
	require.Equal(t, "sonnet-c", replica.GetModelMapping()["claude-sonnet"])

	// 母本不受副本重建影响。
	require.Equal(t, "sonnet-a", master.GetModelMapping()["claude-sonnet"])
}

func TestAccountPrepareSharedSnapshot_MappingLengthChangeRebuilds(t *testing.T) {
	rawMapping := map[string]any{"claude-sonnet": "sonnet-a"}
	master := &Account{
		Platform:    PlatformAnthropic,
		Credentials: map[string]any{"model_mapping": rawMapping},
	}
	master.PrepareSharedSnapshot()

	replica := *master
	rawMapping["claude-opus"] = "opus-a"
	require.Equal(t, "opus-a", replica.GetModelMapping()["claude-opus"])
}

func TestAccountPrepareSharedSnapshot_NilAccountAndNoMapping(t *testing.T) {
	var nilAccount *Account
	require.NotPanics(t, nilAccount.PrepareSharedSnapshot)

	master := &Account{Platform: PlatformAnthropic}
	master.PrepareSharedSnapshot()
	replica := *master
	require.Nil(t, replica.GetModelMapping())
	require.Equal(t, "claude-sonnet", replica.GetMappedModel("claude-sonnet"))
}
