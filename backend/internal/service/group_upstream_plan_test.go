//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUpstreamPlanCode(t *testing.T) {
	require.Equal(t, "pro", NormalizeUpstreamPlanCode(" Pro "))
	require.Equal(t, "supergrok", NormalizeUpstreamPlanCode("SuperGrok"))
	require.Equal(t, "", NormalizeUpstreamPlanCode("  "))
}

func TestNormalizeGroupUpstreamPlans_RejectsDuplicate(t *testing.T) {
	_, err := NormalizeGroupUpstreamPlans(map[string][]GroupUpstreamPlanOption{
		"openai": {
			{Code: "pro", Label: "Pro"},
			{Code: "PRO", Label: "Pro2"},
		},
	})
	require.Error(t, err)
}

func TestDefaultSeedHasExpectedPlatforms(t *testing.T) {
	seed := DefaultGroupUpstreamPlansSeed()
	require.NotEmpty(t, seed[PlatformOpenAI])
	require.NotEmpty(t, seed[PlatformGrok])
	require.NotEmpty(t, seed[PlatformAntigravity])
	require.Empty(t, seed[PlatformAnthropic])
	require.Empty(t, seed[PlatformGemini])
}

type memorySettingRepo struct {
	values map[string]string
}

func (m *memorySettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (m *memorySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if m.values == nil {
		return "", ErrSettingNotFound
	}
	v, ok := m.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}
func (m *memorySettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *memorySettingRepo) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(m.values))
	for k, v := range m.values {
		out[k] = v
	}
	return out, nil
}
func (m *memorySettingRepo) Set(_ context.Context, key, value string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	m.values[key] = value
	return nil
}
func (m *memorySettingRepo) SetMultiple(_ context.Context, updates map[string]string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	for k, v := range updates {
		m.values[k] = v
	}
	return nil
}
func (m *memorySettingRepo) Delete(context.Context, string) error { return nil }

func TestGetGroupUpstreamPlans_SeedsWhenEmpty(t *testing.T) {
	repo := &memorySettingRepo{values: map[string]string{}}
	svc := &SettingService{settingRepo: repo}
	plans, err := svc.GetGroupUpstreamPlans(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, plans[PlatformOpenAI])
	// 应写回 DB
	require.NotEmpty(t, repo.values[SettingKeyGroupUpstreamPlans])
}

func TestValidateGroupUpstreamPlan(t *testing.T) {
	raw, err := MarshalGroupUpstreamPlansJSON(DefaultGroupUpstreamPlansSeed())
	require.NoError(t, err)
	repo := &memorySettingRepo{values: map[string]string{SettingKeyGroupUpstreamPlans: raw}}
	svc := &SettingService{settingRepo: repo}

	code, err := svc.ValidateGroupUpstreamPlan(context.Background(), PlatformOpenAI, "Pro")
	require.NoError(t, err)
	require.Equal(t, "pro", code)

	_, err = svc.ValidateGroupUpstreamPlan(context.Background(), PlatformOpenAI, "not-a-plan")
	require.Error(t, err)

	_, err = svc.ValidateGroupUpstreamPlan(context.Background(), PlatformComposite, "pro")
	require.Error(t, err)

	code, err = svc.ValidateGroupUpstreamPlan(context.Background(), PlatformOpenAI, "")
	require.NoError(t, err)
	require.Equal(t, "", code)
}
