package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userSubscriptionRepository struct {
	client *dbent.Client
REDACTED

func NewUserSubscriptionRepository(client *dbent.Client) service.UserSubscriptionRepository {
	return &userSubscriptionRepository{client: clientREDACTED
REDACTED

func (r *userSubscriptionRepository) Create(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return nil
REDACTED

	builder := r.client.UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetExpiresAt(sub.ExpiresAt).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNillableAssignedBy(sub.AssignedBy)

	if sub.StartsAt.IsZero() {
		builder.SetStartsAt(time.Now())
REDACTED else {
		builder.SetStartsAt(sub.StartsAt)
REDACTED
	if sub.Status != "" {
		builder.SetStatus(sub.Status)
REDACTED
	if !sub.AssignedAt.IsZero() {
		builder.SetAssignedAt(sub.AssignedAt)
REDACTED
	// Keep compatibility with historical behavior: always store notes as a string value.
	builder.SetNotes(sub.Notes)

	created, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, created)
REDACTED
	return translatePersistenceError(err, nil, service.ErrSubscriptionAlreadyExists)
REDACTED

func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	m, err := r.client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED
	return userSubscriptionEntityToService(m), nil
REDACTED

func (r *userSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	m, err := r.client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED
	return userSubscriptionEntityToService(m), nil
REDACTED

func (r *userSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	m, err := r.client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED
	return userSubscriptionEntityToService(m), nil
REDACTED

func (r *userSubscriptionRepository) Update(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return nil
REDACTED

	builder := r.client.UserSubscription.UpdateOneID(sub.ID).
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNillableAssignedBy(sub.AssignedBy).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes)

	updated, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, updated)
		return nil
REDACTED
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionAlreadyExists)
REDACTED

func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	// Match GORM semantics: deleting a missing row is not an error.
	_, err := r.client.UserSubscription.Delete().Where(usersubscription.IDEQ(id)).Exec(ctx)
	return err
REDACTED

func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	subs, err := r.client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID)).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return userSubscriptionEntitiesToService(subs), nil
REDACTED

func (r *userSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	subs, err := r.client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return userSubscriptionEntitiesToService(subs), nil
REDACTED

func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	q := r.client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	subs, err := q.
		WithUser().
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *userSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	q := r.client.UserSubscription.Query()
	if userID != nil {
		q = q.Where(usersubscription.UserIDEQ(*userID))
REDACTED
	if groupID != nil {
		q = q.Where(usersubscription.GroupIDEQ(*groupID))
REDACTED
	if status != "" {
		q = q.Where(usersubscription.StatusEQ(status))
REDACTED

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	subs, err := q.
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *userSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	return r.client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Exist(ctx)
REDACTED

func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	_, err := r.client.UserSubscription.UpdateOneID(subscriptionID).
		SetExpiresAt(newExpiresAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	_, err := r.client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(status).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	_, err := r.client.UserSubscription.UpdateOneID(subscriptionID).
		SetNotes(notes).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	_, err := r.client.UserSubscription.UpdateOneID(id).
		SetDailyWindowStart(start).
		SetWeeklyWindowStart(start).
		SetMonthlyWindowStart(start).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	_, err := r.client.UserSubscription.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	_, err := r.client.UserSubscription.UpdateOneID(id).
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	_, err := r.client.UserSubscription.UpdateOneID(id).
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	_, err := r.client.UserSubscription.UpdateOneID(id).
		AddDailyUsageUsd(costUSD).
		AddWeeklyUsageUsd(costUSD).
		AddMonthlyUsageUsd(costUSD).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
REDACTED

func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	n, err := r.client.UserSubscription.Update().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	return int64(n), err
REDACTED

// Extra repository helpers (currently used only by integration tests).

func (r *userSubscriptionRepository) ListExpired(ctx context.Context) ([]service.UserSubscription, error) {
	subs, err := r.client.UserSubscription.Query().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return userSubscriptionEntitiesToService(subs), nil
REDACTED

func (r *userSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	count, err := r.client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
REDACTED

func (r *userSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	count, err := r.client.UserSubscription.Query().
		Where(
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		Count(ctx)
	return int64(count), err
REDACTED

func (r *userSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	n, err := r.client.UserSubscription.Delete().Where(usersubscription.GroupIDEQ(groupID)).Exec(ctx)
	return int64(n), err
REDACTED

func userSubscriptionEntityToService(m *dbent.UserSubscription) *service.UserSubscription {
	if m == nil {
		return nil
REDACTED
	out := &service.UserSubscription{
		ID:                 m.ID,
		UserID:             m.UserID,
		GroupID:            m.GroupID,
		StartsAt:           m.StartsAt,
		ExpiresAt:          m.ExpiresAt,
		Status:             m.Status,
		DailyWindowStart:   m.DailyWindowStart,
		WeeklyWindowStart:  m.WeeklyWindowStart,
		MonthlyWindowStart: m.MonthlyWindowStart,
		DailyUsageUSD:      m.DailyUsageUsd,
		WeeklyUsageUSD:     m.WeeklyUsageUsd,
		MonthlyUsageUSD:    m.MonthlyUsageUsd,
		AssignedBy:         m.AssignedBy,
		AssignedAt:         m.AssignedAt,
		Notes:              derefString(m.Notes),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
REDACTED
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
REDACTED
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
REDACTED
	if m.Edges.AssignedByUser != nil {
		out.AssignedByUser = userEntityToService(m.Edges.AssignedByUser)
REDACTED
	return out
REDACTED

func userSubscriptionEntitiesToService(models []*dbent.UserSubscription) []service.UserSubscription {
	out := make([]service.UserSubscription, 0, len(models))
	for i := range models {
		if s := userSubscriptionEntityToService(models[i]); s != nil {
			out = append(out, *s)
	REDACTED
REDACTED
	return out
REDACTED

func applyUserSubscriptionEntityToService(dst *service.UserSubscription, src *dbent.UserSubscription) {
	if dst == nil || src == nil {
		return
REDACTED
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
REDACTED
