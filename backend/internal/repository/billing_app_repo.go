package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/billingapp"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type billingAppRepository struct {
	client *ent.Client
}

// NewBillingAppRepository 创建接入方身份仓储。
func NewBillingAppRepository(client *ent.Client) service.BillingAppRepository {
	return &billingAppRepository{client: client}
}

func (r *billingAppRepository) GetByAppID(ctx context.Context, appID string) (*service.BillingApp, error) {
	m, err := r.client.BillingApp.Query().Where(billingapp.AppIDEQ(appID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrBillingAppNotFound
		}
		return nil, err
	}
	return toBillingApp(m), nil
}

func (r *billingAppRepository) Create(ctx context.Context, app *service.BillingApp) (*service.BillingApp, error) {
	m, err := r.client.BillingApp.Create().
		SetAppID(app.AppID).
		SetAppName(app.AppName).
		SetEnabled(app.Enabled).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, service.ErrLedgerRequestConflict
		}
		return nil, err
	}
	return toBillingApp(m), nil
}

func (r *billingAppRepository) SetEnabled(ctx context.Context, appID string, enabled bool) error {
	n, err := r.client.BillingApp.Update().
		Where(billingapp.AppIDEQ(appID)).
		SetEnabled(enabled).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrBillingAppNotFound
	}
	return nil
}

func (r *billingAppRepository) BumpTokenVersion(ctx context.Context, appID string) (int, error) {
	m, err := r.client.BillingApp.Query().Where(billingapp.AppIDEQ(appID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, service.ErrBillingAppNotFound
		}
		return 0, err
	}
	newVersion := m.TokenVersion + 1
	if _, err := m.Update().SetTokenVersion(newVersion).Save(ctx); err != nil {
		return 0, err
	}
	return newVersion, nil
}

func (r *billingAppRepository) Delete(ctx context.Context, appID string) error {
	n, err := r.client.BillingApp.Delete().Where(billingapp.AppIDEQ(appID)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrBillingAppNotFound
	}
	return nil
}

func (r *billingAppRepository) List(ctx context.Context) ([]*service.BillingApp, error) {
	ms, err := r.client.BillingApp.Query().Order(ent.Asc(billingapp.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.BillingApp, 0, len(ms))
	for _, m := range ms {
		out = append(out, toBillingApp(m))
	}
	return out, nil
}

func toBillingApp(m *ent.BillingApp) *service.BillingApp {
	return &service.BillingApp{
		ID:           m.ID,
		AppID:        m.AppID,
		AppName:      m.AppName,
		Enabled:      m.Enabled,
		TokenVersion: m.TokenVersion,
	}
}
