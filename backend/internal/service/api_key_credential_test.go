//go:build unit

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHashAPIKeyCredentialMatchesMigrationExpression 钉住摘要算法。
//
// migrations/239 用 encode(sha256(convert_to(key,'UTF8')),'hex') 回填存量行，
// Go 侧必须逐字节一致，否则回填出来的 key_hash 与运行时算出的对不上，
// 所有存量 Key 会在升级后立刻认证失败。
func TestHashAPIKeyCredentialMatchesMigrationExpression(t *testing.T) {
	const key = "sk-0123456789abcdef0123456789abcdef"

	sum := sha256.Sum256([]byte(key))
	require.Equal(t, hex.EncodeToString(sum[:]), HashAPIKeyCredential(key))
	require.Len(t, HashAPIKeyCredential(key), 64)
	require.Equal(t, "", HashAPIKeyCredential(""), "empty key must not hash to a real digest")

	// 摘要必须是确定性的：认证是等值查询，不能带随机盐。
	require.Equal(t, HashAPIKeyCredential(key), HashAPIKeyCredential(key))
	require.NotEqual(t, HashAPIKeyCredential(key), HashAPIKeyCredential(key+"x"))
}

func TestMaskAPIKeyCredential(t *testing.T) {
	require.Equal(t, "", MaskAPIKeyCredential(""))
	require.Equal(t, "sk-012345678...cdef", MaskAPIKeyCredential("sk-0123456789abcdef"))
	// 短到遮不住的 Key 整条打码，不泄露前缀。
	require.Equal(t, "****************", MaskAPIKeyCredential("sk-0123456789abc"))
	// 中段必须消失，只留头 12 位与尾 4 位。
	require.Equal(t, "sk-012345678...ghij", MaskAPIKeyCredential("sk-0123456789abcdefghij"))
	require.NotContains(t, MaskAPIKeyCredential("sk-0123456789abcdefghij"), "9abcdef")
}

// TestMaskAPIKeyCredentialsStripsFullKeyAndHash 覆盖 H6：
// 管理端列表必须在离开 service 之前就把完整 Key 换成掩码，
// 并且不能把摘要顺手下发出去。
func TestMaskAPIKeyCredentialsStripsFullKeyAndHash(t *testing.T) {
	const full = "sk-0123456789abcdef0123456789abcdef"
	keys := []APIKey{
		{ID: 1, Key: full, KeyHash: HashAPIKeyCredential(full)},
		{ID: 2, Key: "", KeyHash: ""},
	}

	maskAPIKeyCredentials(keys)

	require.NotEqual(t, full, keys[0].Key)
	require.NotContains(t, keys[0].Key, "9abcdef0123456789abcdef")
	require.Equal(t, MaskAPIKeyCredential(full), keys[0].Key)
	require.Empty(t, keys[0].KeyHash, "key_hash must not be exposed to the admin API either")
	require.Empty(t, keys[1].Key)
}
