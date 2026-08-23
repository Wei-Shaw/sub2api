package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/developerkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type developerKeyRepository struct {
	client *ent.Client
}

func NewDeveloperKeyRepository(client *ent.Client) service.DeveloperKeyRepository {
	return &developerKeyRepository{client: client}
}

func (r *developerKeyRepository) Create(ctx context.Context, key *service.DeveloperKey, hash string) (*service.DeveloperKey, error) {
	row, err := r.client.DeveloperKey.Create().
		SetUserID(key.UserID).
		SetName(key.Name).
		SetKeyPrefix(key.KeyPrefix).
		SetKeyHash(hash).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return toDeveloperKey(row), nil
}

func (r *developerKeyRepository) ListByUserID(ctx context.Context, userID int64) ([]*service.DeveloperKey, error) {
	rows, err := r.client.DeveloperKey.Query().
		Where(developerkey.UserIDEQ(userID)).
		Order(ent.Desc(developerkey.FieldCreatedAt), ent.Desc(developerkey.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.DeveloperKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDeveloperKey(row))
	}
	return out, nil
}

func (r *developerKeyRepository) DeleteByUserID(ctx context.Context, userID, id int64) error {
	n, err := r.client.DeveloperKey.Delete().
		Where(developerkey.IDEQ(id), developerkey.UserIDEQ(userID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrDeveloperKeyNotFound
	}
	return nil
}

func (r *developerKeyRepository) GetByHash(ctx context.Context, hash string) (*service.DeveloperKey, error) {
	row, err := r.client.DeveloperKey.Query().Where(developerkey.KeyHashEQ(hash)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrDeveloperKeyNotFound
		}
		return nil, err
	}
	return toDeveloperKey(row), nil
}

func (r *developerKeyRepository) TouchLastUsed(ctx context.Context, id int64, at time.Time) error {
	n, err := r.client.DeveloperKey.Update().
		Where(developerkey.IDEQ(id)).
		SetLastUsedAt(at).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrDeveloperKeyNotFound
	}
	return nil
}

func toDeveloperKey(row *ent.DeveloperKey) *service.DeveloperKey {
	if row == nil {
		return nil
	}
	return &service.DeveloperKey{
		ID:         row.ID,
		UserID:     row.UserID,
		Name:       row.Name,
		KeyPrefix:  row.KeyPrefix,
		LastUsedAt: row.LastUsedAt,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
