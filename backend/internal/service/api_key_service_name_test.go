//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 名称仍按 HTML 转义存储（防 XSS），但前端会把已转义的名称原样提交回来，
// 所以转义必须幂等：同一个名称反复保存不能越转越长。

type nameAPIKeyRepoStub struct {
	quotaBaseAPIKeyRepoStub
	key *APIKey
}

func (s *nameAPIKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.key = &clone
	return nil
}

func (s *nameAPIKeyRepoStub) GetByID(context.Context, int64) (*APIKey, error) {
	clone := *s.key
	return &clone, nil
}

func (s *nameAPIKeyRepoStub) Update(_ context.Context, key *APIKey, _ APIKeyUpdateFields) error {
	clone := *key
	s.key = &clone
	return nil
}

func TestAPIKeyName_EscapeIsIdempotentAcrossSaves(t *testing.T) {
	const raw = "Bob's & Co <x>"
	const stored = "Bob&#39;s &amp; Co &lt;x&gt;"

	repo := &nameAPIKeyRepoStub{}
	svc := &APIKeyService{apiKeyRepo: repo, userRepo: &visibilityUserRepo{user: &User{ID: 7}}, cfg: &config.Config{}}

	created, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: raw})
	require.NoError(t, err)
	require.Equal(t, stored, created.Name)

	// The edit form sends back either the stored (escaped) value or the decoded one.
	for i, name := range []string{repo.key.Name, raw, repo.key.Name} {
		updated, err := svc.Update(context.Background(), 1, 7, UpdateAPIKeyRequest{Name: &name})
		require.NoError(t, err)
		require.Equal(t, stored, updated.Name, "save #%d", i+1)
	}
	require.Equal(t, stored, repo.key.Name)
}
