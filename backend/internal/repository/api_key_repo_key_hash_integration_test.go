//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

// APIKeyKeyHashSuite 覆盖 M1 两阶段迁移第 1 阶段的仓储行为。
type APIKeyKeyHashSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *apiKeyRepository
}

func (s *APIKeyKeyHashSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = newAPIKeyRepositoryWithSQL(s.client, tx)
}

func TestAPIKeyKeyHashSuite(t *testing.T) {
	suite.Run(t, new(APIKeyKeyHashSuite))
}

func (s *APIKeyKeyHashSuite) mustCreateUser(email string) *dbent.User {
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SetUsername(email).
		Save(s.ctx)
	s.Require().NoError(err)
	return u
}

// TestCreatePersistsKeyHash：新建 Key 必须同时写入摘要列。
func (s *APIKeyKeyHashSuite) TestCreatePersistsKeyHash() {
	user := s.mustCreateUser("keyhash-create@test.com")
	const plain = "sk-keyhash-create-0123456789abcdef"

	key := &service.APIKey{UserID: user.ID, Key: plain, Name: "hash", Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(s.ctx, key))

	row, err := s.client.APIKey.Get(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.HashAPIKeyCredential(plain), row.KeyHash)
	s.Require().Len(row.KeyHash, 64)
}

// TestGetByKeyForAuthUsesHashColumn：认证查询必须能通过摘要命中，
// 且明文被改坏时仍能命中——证明查询确实走的是 key_hash 而不是 key。
func (s *APIKeyKeyHashSuite) TestGetByKeyForAuthUsesHashColumn() {
	user := s.mustCreateUser("keyhash-auth@test.com")
	const plain = "sk-keyhash-auth-0123456789abcdef"

	key := &service.APIKey{UserID: user.ID, Key: plain, Name: "hash", Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(s.ctx, key))

	found, err := s.repo.GetByKeyForAuth(s.ctx, plain)
	s.Require().NoError(err)
	s.Require().Equal(key.ID, found.ID)

	// 把明文列改成别的值，摘要不变：认证仍应命中。
	_, err = s.client.APIKey.UpdateOneID(key.ID).SetKey("sk-rotated-plaintext-value-000001").Save(s.ctx)
	s.Require().NoError(err)

	found, err = s.repo.GetByKeyForAuth(s.ctx, plain)
	s.Require().NoError(err, "auth lookup must resolve through key_hash, not the plaintext column")
	s.Require().Equal(key.ID, found.ID)
}

// TestGetByKeyForAuthFallsBackForLegacyRowsWithoutHash：
// 滚动升级窗口里老版本二进制写入的行 key_hash 为空串，必须仍能认证成功，
// 否则升级期间新建的 Key 会直接失效。
func (s *APIKeyKeyHashSuite) TestGetByKeyForAuthFallsBackForLegacyRowsWithoutHash() {
	user := s.mustCreateUser("keyhash-legacy@test.com")
	const plain = "sk-keyhash-legacy-0123456789abcd"

	key := &service.APIKey{UserID: user.ID, Key: plain, Name: "legacy", Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(s.ctx, key))
	_, err := s.client.APIKey.UpdateOneID(key.ID).SetKeyHash("").Save(s.ctx)
	s.Require().NoError(err)

	found, err := s.repo.GetByKeyForAuth(s.ctx, plain)
	s.Require().NoError(err)
	s.Require().Equal(key.ID, found.ID)
}

// TestGetByKeyForAuthRejectsUnknownKey：未知凭据不得命中任何行，
// 尤其不能因为空串摘要而误匹配到 key_hash 为空的历史行。
func (s *APIKeyKeyHashSuite) TestGetByKeyForAuthRejectsUnknownKey() {
	user := s.mustCreateUser("keyhash-unknown@test.com")
	key := &service.APIKey{
		UserID: user.ID, Key: "sk-keyhash-known-0123456789abcdef",
		Name: "known", Status: service.StatusActive,
	}
	s.Require().NoError(s.repo.Create(s.ctx, key))
	_, err := s.client.APIKey.UpdateOneID(key.ID).SetKeyHash("").Save(s.ctx)
	s.Require().NoError(err)

	_, err = s.repo.GetByKeyForAuth(s.ctx, "sk-not-a-real-key-000000000000")
	s.Require().ErrorIs(err, service.ErrAPIKeyNotFound)

	_, err = s.repo.GetByKeyForAuth(s.ctx, "")
	s.Require().ErrorIs(err, service.ErrAPIKeyNotFound)
}

// TestDeleteRewritesKeyHashTombstone：墓碑必须同时改写 key 与 key_hash，
// 否则重复删除会撞上 key_hash 的部分唯一索引。
func (s *APIKeyKeyHashSuite) TestDeleteRewritesKeyHashTombstone() {
	user := s.mustCreateUser("keyhash-delete@test.com")
	const plain = "sk-keyhash-delete-0123456789abc"

	key := &service.APIKey{UserID: user.ID, Key: plain, Name: "del", Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(s.ctx, key))
	s.Require().NoError(s.repo.DeleteWithAudit(s.ctx, key.ID))

	row, err := s.client.APIKey.Query().Where(apikey.IDEQ(key.ID)).Only(mixins.SkipSoftDelete(s.ctx))
	s.Require().NoError(err)
	s.Require().NotEqual(plain, row.Key)
	s.Require().NotEqual(service.HashAPIKeyCredential(plain), row.KeyHash)
	s.Require().Equal(service.HashAPIKeyCredential(row.Key), row.KeyHash)

	// 同一明文可以再次创建，说明唯一约束确实被墓碑释放了。
	reborn := &service.APIKey{UserID: user.ID, Key: plain, Name: "reborn", Status: service.StatusActive}
	s.Require().NoError(s.repo.Create(s.ctx, reborn))

	// 旧摘要不能再解析到任何活跃 Key 之外的行。
	found, err := s.repo.GetByKeyForAuth(s.ctx, plain)
	s.Require().NoError(err)
	s.Require().Equal(reborn.ID, found.ID)
}
