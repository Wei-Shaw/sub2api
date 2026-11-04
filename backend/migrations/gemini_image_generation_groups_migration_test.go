package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnableGeminiImageGenerationGroupsMigration(t *testing.T) {
	content, err := FS.ReadFile("246_enable_gemini_image_generation_groups.sql")
	require.NoError(t, err)

	var statements []string
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			statements = append(statements, line)
		}
	}
	sql := strings.Join(strings.Fields(strings.Join(statements, " ")), " ")
	require.Equal(t,
		"UPDATE groups SET allow_image_generation = true WHERE platform IN ('gemini', 'antigravity') AND allow_image_generation = false;",
		sql)
}
