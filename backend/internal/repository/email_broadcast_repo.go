package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/emailbroadcast"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type emailBroadcastRepository struct {
	client *dbent.Client
}

// NewEmailBroadcastRepository wires an ent-backed EmailBroadcastRepository.
func NewEmailBroadcastRepository(client *dbent.Client) service.EmailBroadcastRepository {
	return &emailBroadcastRepository{client: client}
}

func (r *emailBroadcastRepository) Create(ctx context.Context, b *service.EmailBroadcast) error {
	client := clientFromContext(ctx, r.client)
	builder := client.EmailBroadcast.Create().
		SetSubject(b.Subject).
		SetBody(b.Body).
		SetBodyFormat(b.BodyFormat).
		SetRecipientsMode(b.RecipientsMode).
		SetStatus(b.Status).
		SetTotalCount(b.TotalCount).
		SetSuccessCount(b.SuccessCount).
		SetFailedCount(b.FailedCount)

	if len(b.RecipientUserIDs) > 0 {
		builder.SetRecipientUserIds(b.RecipientUserIDs)
	}
	if b.CreatedBy != nil {
		builder.SetCreatedBy(*b.CreatedBy)
	}
	if b.ErrorMessage != nil {
		builder.SetErrorMessage(*b.ErrorMessage)
	}
	if b.StartedAt != nil {
		builder.SetStartedAt(*b.StartedAt)
	}
	if b.FinishedAt != nil {
		builder.SetFinishedAt(*b.FinishedAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	applyEmailBroadcastEntityToService(b, created)
	return nil
}

func (r *emailBroadcastRepository) GetByID(ctx context.Context, id int64) (*service.EmailBroadcast, error) {
	m, err := r.client.EmailBroadcast.Query().
		Where(emailbroadcast.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrEmailBroadcastNotFound, nil)
	}
	return emailBroadcastEntityToService(m), nil
}

func (r *emailBroadcastRepository) UpdateStatus(ctx context.Context, id int64, patch service.EmailBroadcastStatusUpdate) error {
	client := clientFromContext(ctx, r.client)
	upd := client.EmailBroadcast.UpdateOneID(id)

	if patch.Status != nil {
		upd.SetStatus(*patch.Status)
	}
	if patch.TotalCount != nil {
		upd.SetTotalCount(*patch.TotalCount)
	}
	if patch.SuccessCount != nil {
		upd.SetSuccessCount(*patch.SuccessCount)
	}
	if patch.FailedCount != nil {
		upd.SetFailedCount(*patch.FailedCount)
	}
	if patch.ErrorMessage != nil {
		upd.SetErrorMessage(*patch.ErrorMessage)
	}
	if patch.StartedAt != nil {
		upd.SetStartedAt(*patch.StartedAt)
	}
	if patch.FinishedAt != nil {
		upd.SetFinishedAt(*patch.FinishedAt)
	}

	_, err := upd.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrEmailBroadcastNotFound, nil)
	}
	return nil
}

func (r *emailBroadcastRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.EmailBroadcast.Delete().Where(emailbroadcast.IDEQ(id)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrEmailBroadcastNotFound
	}
	return nil
}

func (r *emailBroadcastRepository) List(ctx context.Context, params service.EmailBroadcastListParams) (*service.EmailBroadcastListResult, error) {
	q := r.client.EmailBroadcast.Query()
	if params.Status != "" {
		q = q.Where(emailbroadcast.StatusEQ(params.Status))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, err
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	items, err := q.
		Order(dbent.Desc(emailbroadcast.FieldCreatedAt), dbent.Desc(emailbroadcast.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.EmailBroadcast, 0, len(items))
	for _, m := range items {
		out = append(out, *emailBroadcastEntityToService(m))
	}

	return &service.EmailBroadcastListResult{
		Items:    out,
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func emailBroadcastEntityToService(m *dbent.EmailBroadcast) *service.EmailBroadcast {
	if m == nil {
		return nil
	}
	out := &service.EmailBroadcast{
		ID:               m.ID,
		Subject:          m.Subject,
		Body:             m.Body,
		BodyFormat:       m.BodyFormat,
		RecipientsMode:   m.RecipientsMode,
		RecipientUserIDs: append([]int64(nil), m.RecipientUserIds...),
		Status:           m.Status,
		TotalCount:       m.TotalCount,
		SuccessCount:     m.SuccessCount,
		FailedCount:      m.FailedCount,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
	if m.ErrorMessage != nil {
		v := *m.ErrorMessage
		out.ErrorMessage = &v
	}
	if m.CreatedBy != nil {
		v := *m.CreatedBy
		out.CreatedBy = &v
	}
	if m.StartedAt != nil {
		v := *m.StartedAt
		out.StartedAt = &v
	}
	if m.FinishedAt != nil {
		v := *m.FinishedAt
		out.FinishedAt = &v
	}
	return out
}

func applyEmailBroadcastEntityToService(b *service.EmailBroadcast, m *dbent.EmailBroadcast) {
	if b == nil || m == nil {
		return
	}
	b.ID = m.ID
	b.CreatedAt = m.CreatedAt
	b.UpdatedAt = m.UpdatedAt
}
