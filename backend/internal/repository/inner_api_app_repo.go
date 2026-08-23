package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/innerapiapp"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type innerAPIAppRepository struct {
	client *ent.Client
}

// NewInnerAPIAppRepository 创建接入方身份仓储。
func NewInnerAPIAppRepository(client *ent.Client) service.InnerAPIAppRepository {
	return &innerAPIAppRepository{client: client}
}

func (r *innerAPIAppRepository) GetByAppID(ctx context.Context, appID string) (*service.InnerAPIApp, error) {
	m, err := r.client.InnerAPIApp.Query().Where(innerapiapp.AppIDEQ(appID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrInnerAPIAppNotFound
		}
		return nil, err
	}
	return toInnerAPIApp(m), nil
}

func (r *innerAPIAppRepository) Create(ctx context.Context, app *service.InnerAPIApp) (*service.InnerAPIApp, error) {
	m, err := r.client.InnerAPIApp.Create().
		SetAppID(app.AppID).
		SetAppName(app.AppName).
		SetEnabled(app.Enabled).
		SetPermissions(app.Permissions).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, service.ErrLedgerRequestConflict
		}
		return nil, err
	}
	return toInnerAPIApp(m), nil
}

func (r *innerAPIAppRepository) SetEnabled(ctx context.Context, appID string, enabled bool) error {
	n, err := r.client.InnerAPIApp.Update().
		Where(innerapiapp.AppIDEQ(appID)).
		SetEnabled(enabled).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrInnerAPIAppNotFound
	}
	return nil
}

func (r *innerAPIAppRepository) SetPermissions(ctx context.Context, appID string, permissions []string) error {
	n, err := r.client.InnerAPIApp.Update().
		Where(innerapiapp.AppIDEQ(appID)).
		SetPermissions(permissions).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrInnerAPIAppNotFound
	}
	return nil
}

func (r *innerAPIAppRepository) BumpTokenVersion(ctx context.Context, appID string) (int, error) {
	m, err := r.client.InnerAPIApp.Query().Where(innerapiapp.AppIDEQ(appID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, service.ErrInnerAPIAppNotFound
		}
		return 0, err
	}
	newVersion := m.TokenVersion + 1
	if _, err := m.Update().SetTokenVersion(newVersion).Save(ctx); err != nil {
		return 0, err
	}
	return newVersion, nil
}

func (r *innerAPIAppRepository) Delete(ctx context.Context, appID string) error {
	n, err := r.client.InnerAPIApp.Delete().Where(innerapiapp.AppIDEQ(appID)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrInnerAPIAppNotFound
	}
	return nil
}

func (r *innerAPIAppRepository) List(ctx context.Context) ([]*service.InnerAPIApp, error) {
	ms, err := r.client.InnerAPIApp.Query().Order(ent.Asc(innerapiapp.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.InnerAPIApp, 0, len(ms))
	for _, m := range ms {
		out = append(out, toInnerAPIApp(m))
	}
	return out, nil
}

func toInnerAPIApp(m *ent.InnerAPIApp) *service.InnerAPIApp {
	return &service.InnerAPIApp{
		ID:           m.ID,
		AppID:        m.AppID,
		AppName:      m.AppName,
		Enabled:      m.Enabled,
		TokenVersion: m.TokenVersion,
		Permissions:  append([]string(nil), m.Permissions...),
	}
}
