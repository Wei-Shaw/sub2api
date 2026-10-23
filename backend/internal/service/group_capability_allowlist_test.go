//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupCapabilityAllowlist(t *testing.T) {
	cfg := GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{" Daybreak_Red ", "daybreak_blue", "daybreak_blue", ""}, ServiceTiers: []string{"ULTRAFAST", "priority"}}
	require.Equal(t, GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue", "daybreak_red"}, ServiceTiers: []string{"priority", "ultrafast"}}, normalizeGroupCapabilityAllowlist(PlatformOpenAI, cfg))
	require.Equal(t, GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue", "daybreak_red"}, ServiceTiers: []string{"priority", "ultrafast"}}, normalizeGroupCapabilityAllowlist(PlatformComposite, cfg))
	require.Equal(t, GroupCapabilityAllowlist{}, normalizeGroupCapabilityAllowlist(PlatformAnthropic, cfg), "非 openai/composite 平台归零")
	require.Equal(t, GroupCapabilityAllowlist{Enabled: true}, normalizeGroupCapabilityAllowlist(PlatformOpenAI, GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{" "}}))
}

func TestValidateGroupCapabilityAllowlist(t *testing.T) {
	require.NoError(t, validateGroupCapabilityAllowlist(GroupCapabilityAllowlist{}, true))
	require.NoError(t, validateGroupCapabilityAllowlist(GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}}, true))
	require.NoError(t, validateGroupCapabilityAllowlist(GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue", "daybreak_red"}, ServiceTiers: []string{"priority", "ultrafast"}}, false))
	require.NoError(t, validateGroupCapabilityAllowlist(GroupCapabilityAllowlist{Enabled: false}, true), "关闭时不与强制 Fast 冲突")

	for name, cfg := range map[string]GroupCapabilityAllowlist{
		"unknown program":              {Enabled: true, CyberPrograms: []string{"daybreak_green"}},
		"unknown tier":                 {Enabled: true, ServiceTiers: []string{"flex"}},
		"standard is not configurable": {Enabled: true, CyberPrograms: []string{"standard"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateGroupCapabilityAllowlist(cfg, false)
			require.Error(t, err)
			appErr := infraerrors.FromError(err)
			require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
			require.Equal(t, "INVALID_CAPABILITY_ALLOWLIST", appErr.Reason)
		})
	}
	t.Run("force fast conflict", func(t *testing.T) {
		err := validateGroupCapabilityAllowlist(GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ultrafast"}}, true)
		require.Error(t, err)
		require.Equal(t, "INVALID_CAPABILITY_ALLOWLIST", infraerrors.FromError(err).Reason)
		require.Contains(t, err.Error(), "force OpenAI Fast")
		require.NoError(t, validateGroupCapabilityAllowlist(GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ultrafast"}}, false))
	})
}

func TestGroupCapabilityAllows(t *testing.T) {
	blue := OpenAIAccountRequirement{CyberProgram: "daybreak_blue"}
	red := OpenAIAccountRequirement{CyberProgram: "daybreak_red"}
	ultrafast := OpenAIAccountRequirement{ServiceTier: "ultrafast"}
	fast := OpenAIAccountRequirement{ServiceTier: "priority"}
	none := OpenAIAccountRequirement{}

	disabled := &Group{Platform: PlatformOpenAI}
	for _, req := range []OpenAIAccountRequirement{blue, red, ultrafast, fast, none} {
		require.Empty(t, disabled.CapabilityAllows(req), "关闭时放行一切")
	}
	var nilGroup *Group
	require.Empty(t, nilGroup.CapabilityAllows(blue))

	empty := &Group{Platform: PlatformOpenAI, CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true}}
	require.Equal(t, "daybreak_blue", empty.CapabilityAllows(blue))
	require.Equal(t, "ultrafast", empty.CapabilityAllows(ultrafast))
	require.Equal(t, "priority", empty.CapabilityAllows(fast))
	require.Empty(t, empty.CapabilityAllows(none), "standard / 无需求始终放行")

	fastOnly := &Group{Platform: PlatformOpenAI, CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}}}
	require.Empty(t, fastOnly.CapabilityAllows(fast))
	require.Equal(t, "ultrafast", fastOnly.CapabilityAllows(ultrafast))
	require.Equal(t, "daybreak_blue", fastOnly.CapabilityAllows(blue))
	require.Equal(t, "daybreak_blue", fastOnly.CapabilityAllows(OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "priority"}), "先报 Daybreak")
	require.Equal(t, "ultrafast", fastOnly.CapabilityAllows(OpenAIAccountRequirement{ServiceTier: "ultrafast"}))

	blueOnly := &Group{Platform: PlatformOpenAI, CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue"}, ServiceTiers: []string{"priority", "ultrafast"}}}
	require.Empty(t, blueOnly.CapabilityAllows(blue))
	require.Equal(t, "daybreak_red", blueOnly.CapabilityAllows(red))
	require.Empty(t, blueOnly.CapabilityAllows(OpenAIAccountRequirement{CyberProgram: "daybreak_blue", ServiceTier: "ultrafast"}))
}

func TestAdminService_CreateGroup_CapabilityAllowlist(t *testing.T) {
	t.Run("normalizes and persists", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{createID: 61}
		svc := &adminServiceImpl{groupRepo: repo}
		_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
			Name: "caps", Platform: PlatformOpenAI, RateMultiplier: 1,
			CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"Daybreak_Blue"}, ServiceTiers: []string{"priority"}},
		})
		require.NoError(t, err)
		require.NotNil(t, repo.created)
		require.Equal(t, GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue"}, ServiceTiers: []string{"priority"}}, repo.created.CapabilityAllowlist)
	})
	t.Run("non openai platform is zeroed", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{createID: 62}
		svc := &adminServiceImpl{groupRepo: repo}
		_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
			Name: "caps-anthropic", Platform: PlatformAnthropic, RateMultiplier: 1,
			CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_green"}},
		})
		require.NoError(t, err)
		require.Equal(t, GroupCapabilityAllowlist{}, repo.created.CapabilityAllowlist)
	})
	t.Run("unknown value rejected", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{createID: 63}
		svc := &adminServiceImpl{groupRepo: repo}
		_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
			Name: "caps-bad", Platform: PlatformOpenAI, RateMultiplier: 1,
			CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_green"}},
		})
		require.Error(t, err)
		require.Equal(t, "INVALID_CAPABILITY_ALLOWLIST", infraerrors.FromError(err).Reason)
		require.Nil(t, repo.created)
	})
	t.Run("force fast conflict rejected", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{createID: 64}
		svc := &adminServiceImpl{groupRepo: repo}
		_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
			Name: "caps-conflict", Platform: PlatformComposite, RateMultiplier: 1, ForceOpenAIFast: true,
			CapabilityAllowlist: GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ultrafast"}},
		})
		require.Error(t, err)
		require.Equal(t, "INVALID_CAPABILITY_ALLOWLIST", infraerrors.FromError(err).Reason)
		require.Nil(t, repo.created)
	})
}

func TestAdminService_UpdateGroup_CapabilityAllowlist(t *testing.T) {
	newRepo := func(group *Group) *groupRepoStubForAdmin {
		return &groupRepoStubForAdmin{getByID: group}
	}
	t.Run("sets allowlist", func(t *testing.T) {
		repo := newRepo(&Group{ID: 7, Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive})
		svc := &adminServiceImpl{groupRepo: repo}
		updated, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{
			CapabilityAllowlist: &GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ULTRAFAST", "priority"}},
		})
		require.NoError(t, err)
		require.Equal(t, GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority", "ultrafast"}}, updated.CapabilityAllowlist)
		require.Equal(t, updated.CapabilityAllowlist, repo.updated.CapabilityAllowlist)
	})
	t.Run("nil input keeps existing", func(t *testing.T) {
		existing := GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue"}, ServiceTiers: []string{"priority"}}
		repo := newRepo(&Group{ID: 7, Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive, CapabilityAllowlist: existing})
		svc := &adminServiceImpl{groupRepo: repo}
		updated, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{Name: "renamed"})
		require.NoError(t, err)
		require.Equal(t, existing, updated.CapabilityAllowlist)
	})
	t.Run("unknown value rejected", func(t *testing.T) {
		repo := newRepo(&Group{ID: 7, Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive})
		svc := &adminServiceImpl{groupRepo: repo}
		_, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{
			CapabilityAllowlist: &GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_green"}},
		})
		require.Error(t, err)
		require.Equal(t, "INVALID_CAPABILITY_ALLOWLIST", infraerrors.FromError(err).Reason)
		require.Nil(t, repo.updated)
	})
	t.Run("enabling force fast later conflicts with stored allowlist", func(t *testing.T) {
		existing := GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ultrafast"}}
		repo := newRepo(&Group{ID: 7, Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive, CapabilityAllowlist: existing})
		svc := &adminServiceImpl{groupRepo: repo}
		forceFast := true
		_, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{ForceOpenAIFast: &forceFast})
		require.Error(t, err)
		require.Equal(t, "INVALID_CAPABILITY_ALLOWLIST", infraerrors.FromError(err).Reason)
		require.Nil(t, repo.updated)
	})
	t.Run("platform change zeroes allowlist", func(t *testing.T) {
		existing := GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}}
		repo := newRepo(&Group{ID: 7, Platform: PlatformOpenAI, RateMultiplier: 1, Status: StatusActive, CapabilityAllowlist: existing})
		svc := &adminServiceImpl{groupRepo: repo}
		updated, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{Platform: PlatformAnthropic})
		require.NoError(t, err)
		require.Equal(t, GroupCapabilityAllowlist{}, updated.CapabilityAllowlist)
	})
}
