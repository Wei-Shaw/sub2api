package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestToLocalizedModelPlazaGroups(t *testing.T) {
	visible := []service.PlazaGroup{
		{
			ID: 1, Name: "标准", Description: "号池", Platform: "anthropic",
			I18n: service.GroupI18n{"en": {Name: "Standard", Description: "Pro + Max pool"}},
		},
		{ID: 2, Name: "无译文", Description: "原描述", Platform: "openai"},
	}

	english := toLocalizedModelPlazaGroups(visible, nil, "en")
	require.Equal(t, "Standard", english[0].Name)
	require.Equal(t, "Pro + Max pool", english[0].Description)
	require.Equal(t, "无译文", english[1].Name)
	require.Equal(t, "原描述", english[1].Description)

	chinese := toLocalizedModelPlazaGroups(visible, nil, "zh")
	require.Equal(t, "标准", chinese[0].Name)
	require.Equal(t, "号池", chinese[0].Description)

	// service 层对象不被改写，同一批分组可继续按别的语言输出
	require.Equal(t, "标准", visible[0].Name)

	raw, err := json.Marshal(english[0])
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotContains(t, decoded, "i18n")
}

func TestLocalizeUserAvailableGroups(t *testing.T) {
	refs := []service.AvailableGroupRef{
		{ID: 1, Name: "标准", Platform: "anthropic", I18n: service.GroupI18n{"en": {Name: "Standard"}}},
		{ID: 2, Name: "无译文", Platform: "anthropic"},
	}
	allowed := map[int64]struct{}{1: {}, 2: {}}

	english := filterUserVisibleGroups(refs, allowed)
	localizeUserAvailableGroups(english, "en")
	require.Equal(t, "Standard", english[0].Name)
	require.Equal(t, "无译文", english[1].Name)

	chinese := filterUserVisibleGroups(refs, allowed)
	localizeUserAvailableGroups(chinese, "zh")
	require.Equal(t, "标准", chinese[0].Name)

	raw, err := json.Marshal(english[0])
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotContains(t, decoded, "i18n")
}

// 用户侧 handler 输出带分组的 DTO 时必须经过 Localized，否则分组名称与描述会
// 停留在原字段，并把译文表原样带给用户。管理端（handler/admin）不在此列。
func TestUserFacingGroupDTOsAreLocalized(t *testing.T) {
	mapperCall := regexp.MustCompile(`dto\.(APIKey|UserSubscription|RedeemCode|UsageLog|Group)FromService[A-Za-z]*\(`)

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		for i, line := range strings.Split(string(source), "\n") {
			if !mapperCall.MatchString(line) {
				continue
			}
			checked++
			require.Containsf(t, line, ".Localized(",
				"%s:%d outputs a group DTO without Localized(Accept-Language)", file, i+1)
		}
	}
	require.NotZero(t, checked, "expected to find user-facing group DTO call sites")
}
