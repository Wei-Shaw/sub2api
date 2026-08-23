//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type developerKeyRepoStub struct {
	nextID int64
	rows   map[int64]*DeveloperKey
	hashes map[int64]string
}

func newDeveloperKeyRepoStub() *developerKeyRepoStub {
	return &developerKeyRepoStub{nextID: 1, rows: map[int64]*DeveloperKey{}, hashes: map[int64]string{}}
}

func (r *developerKeyRepoStub) Create(_ context.Context, key *DeveloperKey, hash string) (*DeveloperKey, error) {
	clone := *key
	clone.ID = r.nextID
	clone.CreatedAt = time.Now()
	clone.UpdatedAt = clone.CreatedAt
	r.nextID++
	r.rows[clone.ID] = &clone
	r.hashes[clone.ID] = hash
	out := clone
	return &out, nil
}

func (r *developerKeyRepoStub) ListByUserID(_ context.Context, userID int64) ([]*DeveloperKey, error) {
	var out []*DeveloperKey
	for _, row := range r.rows {
		if row.UserID == userID {
			clone := *row
			out = append(out, &clone)
		}
	}
	return out, nil
}

func (r *developerKeyRepoStub) DeleteByUserID(_ context.Context, userID, id int64) error {
	row, ok := r.rows[id]
	if !ok || row.UserID != userID {
		return ErrDeveloperKeyNotFound
	}
	delete(r.rows, id)
	delete(r.hashes, id)
	return nil
}

func (r *developerKeyRepoStub) GetByHash(_ context.Context, hash string) (*DeveloperKey, error) {
	for id, stored := range r.hashes {
		if stored == hash {
			clone := *r.rows[id]
			return &clone, nil
		}
	}
	return nil, ErrDeveloperKeyNotFound
}

func (r *developerKeyRepoStub) TouchLastUsed(_ context.Context, id int64, at time.Time) error {
	row, ok := r.rows[id]
	if !ok {
		return ErrDeveloperKeyNotFound
	}
	row.LastUsedAt = &at
	return nil
}

type developerKeyUserRepoStub struct {
	UserRepository
	users map[int64]*User
}

func (r *developerKeyUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, errors.New("not found")
	}
	clone := *user
	return &clone, nil
}

func newDeveloperKeyServiceForTest(repo *developerKeyRepoStub) *DeveloperKeyService {
	return NewDeveloperKeyService(repo, &developerKeyUserRepoStub{users: map[int64]*User{
		7: {ID: 7, Status: StatusActive},
		8: {ID: 8, Status: StatusActive},
	}})
}

func TestDeveloperKeyCreateStoresDigestAndReturnsPlaintextOnce(t *testing.T) {
	repo := newDeveloperKeyRepoStub()
	svc := newDeveloperKeyServiceForTest(repo)

	key, plaintext, err := svc.Create(context.Background(), 7, "Uploader")
	require.NoError(t, err)
	require.NotEmpty(t, plaintext)
	require.Contains(t, plaintext, "dev_")
	require.Equal(t, plaintext[:developerKeyViewChars], key.KeyPrefix)
	require.Equal(t, developerKeyHash(plaintext), repo.hashes[key.ID])
	require.NotEqual(t, plaintext, repo.hashes[key.ID])

	listed, err := svc.List(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, key.KeyPrefix, listed[0].KeyPrefix)
}

func TestDeveloperKeyDeleteIsOwnerScopedAndRevokesAuthentication(t *testing.T) {
	repo := newDeveloperKeyRepoStub()
	svc := newDeveloperKeyServiceForTest(repo)
	key, plaintext, err := svc.Create(context.Background(), 7, "CLI")
	require.NoError(t, err)

	require.ErrorIs(t, svc.Delete(context.Background(), 8, key.ID), ErrDeveloperKeyNotFound)
	authed, err := svc.Authenticate(context.Background(), plaintext)
	require.NoError(t, err)
	require.Equal(t, int64(7), authed.UserID)
	require.NotNil(t, authed.LastUsedAt)

	require.NoError(t, svc.Delete(context.Background(), 7, key.ID))
	_, err = svc.Authenticate(context.Background(), plaintext)
	require.ErrorIs(t, err, ErrDeveloperKeyInvalid)
}

func TestDeveloperKeyAuthenticationRejectsMalformedAndInactiveUser(t *testing.T) {
	repo := newDeveloperKeyRepoStub()
	svc := newDeveloperKeyServiceForTest(repo)
	_, _, err := svc.Create(context.Background(), 0, "bad")
	require.ErrorIs(t, err, ErrDeveloperKeyInvalid)
	_, err = svc.Authenticate(context.Background(), "not-a-key")
	require.ErrorIs(t, err, ErrDeveloperKeyInvalid)

	key, plaintext, err := svc.Create(context.Background(), 7, "inactive later")
	require.NoError(t, err)
	require.NotZero(t, key.ID)
	svc.userRepo = &developerKeyUserRepoStub{users: map[int64]*User{7: {ID: 7, Status: StatusDisabled}}}
	_, err = svc.Authenticate(context.Background(), plaintext)
	require.ErrorIs(t, err, ErrDeveloperKeyInvalid)
}
