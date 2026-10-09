package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupI18n(t *testing.T) {
	t.Run("nil input yields an empty non-nil map", func(t *testing.T) {
		got, err := normalizeGroupI18n(nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})

	t.Run("trims text and drops locales with nothing left", func(t *testing.T) {
		got, err := normalizeGroupI18n(GroupI18n{
			"en":    {Name: "  Standard  ", Description: "\tPro + Max pool\n"},
			"zh":    {Name: "   ", Description: ""},
			"zh-TW": {Description: "僅描述"},
		})
		require.NoError(t, err)
		require.Equal(t, GroupI18n{
			"en":    {Name: "Standard", Description: "Pro + Max pool"},
			"zh-TW": {Description: "僅描述"},
		}, got)
	})

	t.Run("rejects malformed locale codes", func(t *testing.T) {
		for _, locale := range []string{"", "e", "en_US", "en-", "中文", "en us"} {
			_, err := normalizeGroupI18n(GroupI18n{locale: {Name: "x"}})
			require.Errorf(t, err, "locale %q should be rejected", locale)
		}
	})

	t.Run("name length is capped in characters like groups.name", func(t *testing.T) {
		atLimit := strings.Repeat("名", groupI18nNameMaxRunes)
		got, err := normalizeGroupI18n(GroupI18n{"zh": {Name: atLimit}})
		require.NoError(t, err)
		require.Equal(t, atLimit, got["zh"].Name)

		_, err = normalizeGroupI18n(GroupI18n{"zh": {Name: atLimit + "名"}})
		require.Error(t, err)
	})
}

func TestCloneGroupI18nForDuplicate(t *testing.T) {
	source := GroupI18n{
		"en": {Name: "Standard", Description: "Pro + Max pool"},
		"zh": {Name: "标准"},
	}

	got := cloneGroupI18nForDuplicate(source)

	// 副本改了 name；名称译文若照搬，副本与源分组在该语言下会同名。
	require.Equal(t, GroupI18n{"en": {Description: "Pro + Max pool"}}, got)
	require.Equal(t, "Standard", source["en"].Name, "source must not be mutated")
	require.NotNil(t, cloneGroupI18nForDuplicate(nil))
}
