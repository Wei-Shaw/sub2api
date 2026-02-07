//go:build unit

package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateRandomID_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{REDACTED, 100)
	for i := 0; i < 100; i++ {
		id := generateRandomID()
		require.Len(t, id, 12, "ID 长度应为 12")
		_, dup := seen[id]
		require.False(t, dup, "第 %d 次调用生成了重复 ID: %s", i, id)
		seen[id] = struct{REDACTED{REDACTED
REDACTED
REDACTED

func TestGenerateRandomID_Charset(t *testing.T) {
	const validChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	validSet := make(map[byte]struct{REDACTED, len(validChars))
	for i := 0; i < len(validChars); i++ {
		validSet[validChars[i]] = struct{REDACTED{REDACTED
REDACTED

	for i := 0; i < 50; i++ {
		id := generateRandomID()
		for j := 0; j < len(id); j++ {
			_, ok := validSet[id[j]]
			require.True(t, ok, "ID 包含非法字符: %c (ID=%s)", id[j], id)
	REDACTED
REDACTED
REDACTED
