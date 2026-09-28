package repository

import (
	"context"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// userInvitationRepository 基于 redeem_codes 表实现用户自助邀请码的持久化。
type userInvitationRepository struct {
	client *dbent.Client
}

func NewUserInvitationRepository(client *dbent.Client) service.UserInvitationRepository {
	return &userInvitationRepository{client: client}
}

// activeInvitationPredicate 占用名额的邀请码：已使用，或未使用且未过期。
func activeInvitationPredicate(userID int64, now time.Time) predicate.RedeemCode {
	return redeemcode.And(
		redeemcode.CreatedByEQ(userID),
		redeemcode.TypeEQ(service.RedeemTypeInvitation),
		redeemcode.Or(
			redeemcode.StatusEQ(service.StatusUsed),
			redeemcode.ExpiresAtIsNil(),
			redeemcode.ExpiresAtGT(now),
		),
	)
}

func (r *userInvitationRepository) CountActiveInvitationsByCreator(ctx context.Context, userID int64, now time.Time) (int, error) {
	return clientFromContext(ctx, r.client).RedeemCode.Query().
		Where(activeInvitationPredicate(userID, now)).
		Count(ctx)
}

func (r *userInvitationRepository) ListInvitationsByCreator(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if limit <= 0 {
		limit = 100
	}
	codes, err := clientFromContext(ctx, r.client).RedeemCode.Query().
		Where(
			redeemcode.CreatedByEQ(userID),
			redeemcode.TypeEQ(service.RedeemTypeInvitation),
		).
		WithUser().
		Order(dbent.Desc(redeemcode.FieldCreatedAt), dbent.Desc(redeemcode.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return redeemCodeEntitiesToService(codes), nil
}

func (r *userInvitationRepository) CreateInvitationForUser(ctx context.Context, code *service.RedeemCode, maxCodes int, now time.Time) error {
	if code == nil || code.CreatedBy == nil {
		return errors.New("user invitation requires a creator")
	}
	creatorID := *code.CreatedBy

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		txClient = r.client
	}

	// 锁定生成者行，串行化同一用户的并发生成请求，保证名额校验与插入的原子性。
	if _, err := txClient.User.Query().
		Where(user.IDEQ(creatorID)).
		ForUpdate().
		OnlyID(ctx); err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrUserNotFound
		}
		return err
	}

	if maxCodes > 0 {
		count, err := txClient.RedeemCode.Query().
			Where(activeInvitationPredicate(creatorID, now)).
			Count(ctx)
		if err != nil {
			return err
		}
		if count >= maxCodes {
			return service.ErrUserInvitationLimitReached
		}
	}

	created, err := txClient.RedeemCode.Create().
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays).
		SetNillableExpiresAt(code.ExpiresAt).
		SetCreatedBy(creatorID).
		Save(ctx)
	if err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	code.ID = created.ID
	code.CreatedAt = created.CreatedAt
	return nil
}
