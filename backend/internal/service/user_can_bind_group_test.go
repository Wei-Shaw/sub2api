package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserCanBindGroup(t *testing.T) {
	const (
		publicGroupA    int64 = 10
		publicGroupB    int64 = 11
		exclusiveGroupA int64 = 20
	)

	for _, tc := range []struct {
		name        string
		user        User
		groupID     int64
		isExclusive bool
		want        bool
	}{
		{
			name:    "public group is bindable by default",
			user:    User{},
			groupID: publicGroupA,
			want:    true,
		},
		{
			name:        "exclusive group needs an explicit grant",
			user:        User{},
			groupID:     exclusiveGroupA,
			isExclusive: true,
			want:        false,
		},
		{
			name:        "granted exclusive group is bindable",
			user:        User{AllowedGroups: []int64{exclusiveGroupA}},
			groupID:     exclusiveGroupA,
			isExclusive: true,
			want:        true,
		},
		{
			name:    "an unrestricted user keeps every public group even with a grant list",
			user:    User{AllowedGroups: []int64{exclusiveGroupA}},
			groupID: publicGroupA,
			want:    true,
		},
		{
			name:    "a restricted user keeps the public groups listed for them",
			user:    User{RestrictPublicGroups: true, AllowedGroups: []int64{publicGroupA}},
			groupID: publicGroupA,
			want:    true,
		},
		{
			name:    "a restricted user loses the public groups not listed for them",
			user:    User{RestrictPublicGroups: true, AllowedGroups: []int64{publicGroupA}},
			groupID: publicGroupB,
			want:    false,
		},
		{
			name:    "a restricted user with no list keeps no public group",
			user:    User{RestrictPublicGroups: true},
			groupID: publicGroupA,
			want:    false,
		},
		{
			name:        "restricting public groups does not widen exclusive access",
			user:        User{RestrictPublicGroups: true, AllowedGroups: []int64{publicGroupA}},
			groupID:     exclusiveGroupA,
			isExclusive: true,
			want:        false,
		},
		{
			name:        "a restricted user can still hold both kinds of grant",
			user:        User{RestrictPublicGroups: true, AllowedGroups: []int64{publicGroupA, exclusiveGroupA}},
			groupID:     exclusiveGroupA,
			isExclusive: true,
			want:        true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := tc.user
			require.Equal(t, tc.want, user.CanBindGroup(tc.groupID, tc.isExclusive))
		})
	}
}
