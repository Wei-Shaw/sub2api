package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountImportNaming(t *testing.T) {
	rule := &AccountImportNaming{Prefix: "GPT-Free-", StartNumber: 1, Padding: 2}
	name, err := rule.Name(0)
	require.NoError(t, err)
	require.Equal(t, "GPT-Free-01", name)
	name, err = rule.Name(99)
	require.NoError(t, err)
	require.Equal(t, "GPT-Free-100", name)
	for _, invalid := range []AccountImportNaming{
		{Padding: 0}, {Padding: 17}, {Padding: 2, StartNumber: -1},
		{Padding: 2, StartNumber: maxAccountImportSequence},
		{Prefix: strings.Repeat("名", 100), Padding: 2}, {Prefix: "invalid\x00", Padding: 2},
	} {
		_, err := invalid.Name(1)
		require.Error(t, err)
	}
}

func TestAccountImportPoolIDOnlyAssignsOpenAIOAuth(t *testing.T) {
	id := int64(8)
	require.Equal(t, &id, AccountImportPoolID(&id, PlatformOpenAI, AccountTypeOAuth))
	require.Nil(t, AccountImportPoolID(&id, PlatformOpenAI, AccountTypeAPIKey))
	require.Nil(t, AccountImportPoolID(&id, PlatformAnthropic, AccountTypeOAuth))
}
