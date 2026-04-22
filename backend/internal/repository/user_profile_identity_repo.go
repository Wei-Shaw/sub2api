package repository

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	ErrAuthIdentityOwnershipConflict = infraerrors.Conflict(
		"AUTH_IDENTITY_OWNERSHIP_CONFLICT",
		"auth identity already belongs to another user",
	)
	ErrAuthIdentityChannelOwnershipConflict = infraerrors.Conflict(
		"AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT",
		"auth identity channel already belongs to another user",
	)
	ErrAuthIdentityChannelProviderMismatch = infraerrors.BadRequest(
		"AUTH_IDENTITY_CHANNEL_PROVIDER_MISMATCH",
		"auth identity channel provider must match canonical identity",
	)
)

type ProviderGrantReason string

const (
	ProviderGrantReasonSignup    ProviderGrantReason = "signup"
	ProviderGrantReasonFirstBind ProviderGrantReason = "first_bind"
)

type AuthIdentityKey struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
}

type AuthIdentityChannelKey struct {
	ProviderType   string
	ProviderKey    string
	Channel        string
	ChannelAppID   string
	ChannelSubject string
}

type CreateAuthIdentityInput struct {
	UserID          int64
	Canonical       AuthIdentityKey
	Channel         *AuthIdentityChannelKey
	Issuer          *string
	VerifiedAt      *time.Time
	Metadata        map[string]any
	ChannelMetadata map[string]any
}

type BindAuthIdentityInput = CreateAuthIdentityInput

type CreateAuthIdentityResult struct {
	Identity *dbent.AuthIdentity
	Channel  *dbent.AuthIdentityChannel
}

func (r *CreateAuthIdentityResult) IdentityRef() AuthIdentityKey {
	if r == nil || r.Identity == nil {
		return AuthIdentityKey{}
	}
	return AuthIdentityKey{
		ProviderType:    r.Identity.ProviderType,
		ProviderKey:     r.Identity.ProviderKey,
		ProviderSubject: r.Identity.ProviderSubject,
	}
}

func (r *CreateAuthIdentityResult) ChannelRef() *AuthIdentityChannelKey {
	if r == nil || r.Channel == nil {
		return nil
	}
	return &AuthIdentityChannelKey{
		ProviderType:   r.Channel.ProviderType,
		ProviderKey:    r.Channel.ProviderKey,
		Channel:        r.Channel.Channel,
		ChannelAppID:   r.Channel.ChannelAppID,
		ChannelSubject: r.Channel.ChannelSubject,
	}
}

type UserAuthIdentityLookup struct {
	User     *dbent.User
	Identity *dbent.AuthIdentity
	Channel  *dbent.AuthIdentityChannel
}

type ProviderGrantRecordInput struct {
	UserID       int64
	ProviderType string
	GrantReason  ProviderGrantReason
}

type IdentityAdoptionDecisionInput struct {
	PendingAuthSessionID int64
	IdentityID           *int64
	AdoptDisplayName     bool
	AdoptAvatar          bool
}

type sqlQueryExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

var repositoryScopedKeyLocks = newScopedKeyLockRegistry()

type scopedKeyLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*scopedKeyLockEntry
}

type scopedKeyLockEntry struct {
	mu   sync.Mutex
	refs int
}

func newScopedKeyLockRegistry() *scopedKeyLockRegistry {
	return &scopedKeyLockRegistry{
		locks: make(map[string]*scopedKeyLockEntry),
	}
}

func (r *scopedKeyLockRegistry) lock(keys ...string) func() {
	normalized := normalizeLockKeys(keys...)
	if len(normalized) == 0 {
		return func() {}
	}

	entries := make([]*scopedKeyLockEntry, 0, len(normalized))
	r.mu.Lock()
	for _, key := range normalized {
		entry := r.locks[key]
		if entry == nil {
			entry = &scopedKeyLockEntry{}
			r.locks[key] = entry
		}
		entry.refs++
		entries = append(entries, entry)
	}
	r.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
	}

	return func() {
		for i := len(entries) - 1; i >= 0; i-- {
			entries[i].mu.Unlock()
		}

		r.mu.Lock()
		defer r.mu.Unlock()
		for idx, key := range normalized {
			entry := entries[idx]
			entry.refs--
			if entry.refs == 0 {
				delete(r.locks, key)
			}
		}
	}
}

func normalizeLockKeys(keys ...string) []string {
	if len(keys) == 0 {
		return nil
	}

	deduped := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		deduped[trimmed] = struct{}{}
	}
	if len(deduped) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(deduped))
	for key := range deduped {
		normalized = append(normalized, key)
	}
	sort.Strings(normalized)
	return normalized
}

func advisoryLockHash(key string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int64(hasher.Sum64())
}

func lockRepositoryScopedKeys(ctx context.Context, client *dbent.Client, exec sqlQueryExecutor, keys ...string) (func(), error) {
	release := repositoryScopedKeyLocks.lock(keys...)
	normalized := normalizeLockKeys(keys...)
	if len(normalized) == 0 || client == nil || exec == nil || client.Driver().Dialect() != dialect.Postgres {
		return release, nil
	}

	for _, key := range normalized {
		rows, err := exec.QueryContext(ctx, "SELECT pg_advisory_xact_lock($1)", advisoryLockHash(key))
		if err != nil {
			release()
			return nil, err
		}
		_ = rows.Close()
	}
	return release, nil
}

func (r *userRepository) WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *userRepository) CreateAuthIdentity(ctx context.Context, input CreateAuthIdentityInput) (*CreateAuthIdentityResult, error) {
	if err := validateAuthIdentityChannelProviderMatch(input.Canonical, input.Channel); err != nil {
		return nil, err
	}

	client := clientFromContext(ctx, r.client)

	create := client.AuthIdentity.Create().
		SetUserID(input.UserID).
		SetProviderType(strings.TrimSpace(input.Canonical.ProviderType)).
		SetProviderKey(strings.TrimSpace(input.Canonical.ProviderKey)).
		SetProviderSubject(strings.TrimSpace(input.Canonical.ProviderSubject)).
		SetMetadata(copyMetadata(input.Metadata)).
		SetNillableIssuer(input.Issuer).
		SetNillableVerifiedAt(input.VerifiedAt)

	identity, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}

	var channel *dbent.AuthIdentityChannel
	if input.Channel != nil {
		channel, err = client.AuthIdentityChannel.Create().
			SetIdentityID(identity.ID).
			SetProviderType(strings.TrimSpace(input.Channel.ProviderType)).
			SetProviderKey(strings.TrimSpace(input.Channel.ProviderKey)).
			SetChannel(strings.TrimSpace(input.Channel.Channel)).
			SetChannelAppID(strings.TrimSpace(input.Channel.ChannelAppID)).
			SetChannelSubject(strings.TrimSpace(input.Channel.ChannelSubject)).
			SetMetadata(copyMetadata(input.ChannelMetadata)).
			Save(ctx)
		if err != nil {
			return nil, err
		}
	}

	return &CreateAuthIdentityResult{Identity: identity, Channel: channel}, nil
}

func (r *userRepository) GetUserByCanonicalIdentity(ctx context.Context, key AuthIdentityKey) (*UserAuthIdentityLookup, error) {
	identity, err := clientFromContext(ctx, r.client).AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(strings.TrimSpace(key.ProviderType)),
			authidentity.ProviderKeyEQ(strings.TrimSpace(key.ProviderKey)),
			authidentity.ProviderSubjectEQ(strings.TrimSpace(key.ProviderSubject)),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, err
	}

	return &UserAuthIdentityLookup{
		User:     identity.Edges.User,
		Identity: identity,
	}, nil
}

func (r *userRepository) GetUserByChannelIdentity(ctx context.Context, key AuthIdentityChannelKey) (*UserAuthIdentityLookup, error) {
	channel, err := clientFromContext(ctx, r.client).AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ(strings.TrimSpace(key.ProviderType)),
			authidentitychannel.ProviderKeyEQ(strings.TrimSpace(key.ProviderKey)),
			authidentitychannel.ChannelEQ(strings.TrimSpace(key.Channel)),
			authidentitychannel.ChannelAppIDEQ(strings.TrimSpace(key.ChannelAppID)),
			authidentitychannel.ChannelSubjectEQ(strings.TrimSpace(key.ChannelSubject)),
		).
		WithIdentity(func(q *dbent.AuthIdentityQuery) {
			q.WithUser()
		}).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	return &UserAuthIdentityLookup{
		User:     channel.Edges.Identity.Edges.User,
		Identity: channel.Edges.Identity,
		Channel:  channel,
	}, nil
}

func (r *userRepository) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	identities, err := clientFromContext(ctx, r.client).AuthIdentity.Query().
		Where(authidentity.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	records := make([]service.UserAuthIdentityRecord, 0, len(identities))
	for _, identity := range identities {
		if identity == nil {
			continue
		}
		records = append(records, service.UserAuthIdentityRecord{
			ProviderType:    strings.TrimSpace(identity.ProviderType),
			ProviderKey:     strings.TrimSpace(identity.ProviderKey),
			ProviderSubject: strings.TrimSpace(identity.ProviderSubject),
			VerifiedAt:      identity.VerifiedAt,
			Issuer:          identity.Issuer,
			Metadata:        copyMetadata(identity.Metadata),
			CreatedAt:       identity.CreatedAt,
			UpdatedAt:       identity.UpdatedAt,
		})
	}

	return records, nil
}

func (r *userRepository) UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "email" {
		return service.ErrIdentityProviderInvalid
	}

	return r.WithUserProfileIdentityTx(ctx, func(txCtx context.Context) error {
		client := clientFromContext(txCtx, r.client)
		identityIDs, err := client.AuthIdentity.Query().
			Where(
				authidentity.UserIDEQ(userID),
				authidentity.ProviderTypeEQ(provider),
			).
			IDs(txCtx)
		if err != nil {
			return err
		}
		if len(identityIDs) == 0 {
			return nil
		}

		if _, err := client.IdentityAdoptionDecision.Update().
			Where(identityadoptiondecision.IdentityIDIn(identityIDs...)).
			ClearIdentityID().
			Save(txCtx); err != nil {
			return err
		}
		if _, err := client.AuthIdentityChannel.Delete().
			Where(authidentitychannel.IdentityIDIn(identityIDs...)).
			Exec(txCtx); err != nil {
			return err
		}
		_, err = client.AuthIdentity.Delete().
			Where(
				authidentity.UserIDEQ(userID),
				authidentity.ProviderTypeEQ(provider),
			).
			Exec(txCtx)
		return err
	})
}

func (r *userRepository) BindAuthIdentityToUser(ctx context.Context, input BindAuthIdentityInput) (*CreateAuthIdentityResult, error) {
	if err := validateAuthIdentityChannelProviderMatch(input.Canonical, input.Channel); err != nil {
		return nil, err
	}

	var result *CreateAuthIdentityResult
	err := r.WithUserProfileIdentityTx(ctx, func(txCtx context.Context) error {
		client := clientFromContext(txCtx, r.client)
		canonical := input.Canonical

		identityRecords, err := client.AuthIdentity.Query().
			Where(
				authidentity.ProviderTypeEQ(strings.TrimSpace(canonical.ProviderType)),
				authidentity.ProviderKeyIn(compatibleIdentityProviderKeys(canonical.ProviderType, canonical.ProviderKey)...),
				authidentity.ProviderSubjectEQ(strings.TrimSpace(canonical.ProviderSubject)),
			).
			All(txCtx)
		if err != nil {
			return err
		}
		identity := selectOwnedCompatibleIdentity(identityRecords, input.UserID)
		if identity == nil && hasCompatibleIdentityConflict(identityRecords, input.UserID) {
			return ErrAuthIdentityOwnershipConflict
		}
		if identity == nil {
			identity, err = client.AuthIdentity.Create().
				SetUserID(input.UserID).
				SetProviderType(strings.TrimSpace(canonical.ProviderType)).
				SetProviderKey(strings.TrimSpace(canonical.ProviderKey)).
				SetProviderSubject(strings.TrimSpace(canonical.ProviderSubject)).
				SetMetadata(copyMetadata(input.Metadata)).
				SetNillableIssuer(input.Issuer).
				SetNillableVerifiedAt(input.VerifiedAt).
				Save(txCtx)
			if err != nil {
				return err
			}
		} else {
			targetProviderKey := canonicalizeCompatibleIdentityProviderKey(canonical.ProviderType, identity.ProviderKey, canonical.ProviderKey)
			update := client.AuthIdentity.UpdateOneID(identity.ID)
			if targetProviderKey != "" && !strings.EqualFold(targetProviderKey, identity.ProviderKey) {
				update = update.SetProviderKey(targetProviderKey)
			}
			if input.Metadata != nil {
				update = update.SetMetadata(copyMetadata(input.Metadata))
			}
			if input.Issuer != nil {
				update = update.SetIssuer(strings.TrimSpace(*input.Issuer))
			}
			if input.VerifiedAt != nil {
				update = update.SetVerifiedAt(*input.VerifiedAt)
			}
			identity, err = update.Save(txCtx)
			if err != nil {
				return err
			}
		}

		var channel *dbent.AuthIdentityChannel
		if input.Channel != nil {
			channelRecords, err := client.AuthIdentityChannel.Query().
				Where(
					authidentitychannel.ProviderTypeEQ(strings.TrimSpace(input.Channel.ProviderType)),
					authidentitychannel.ProviderKeyIn(compatibleIdentityProviderKeys(input.Channel.ProviderType, input.Channel.ProviderKey)...),
					authidentitychannel.ChannelEQ(strings.TrimSpace(input.Channel.Channel)),
					authidentitychannel.ChannelAppIDEQ(strings.TrimSpace(input.Channel.ChannelAppID)),
					authidentitychannel.ChannelSubjectEQ(strings.TrimSpace(input.Channel.ChannelSubject)),
				).
				WithIdentity().
				All(txCtx)
			if err != nil {
				return err
			}
			channel = selectOwnedCompatibleChannel(channelRecords, input.UserID)
			if channel == nil && hasCompatibleChannelConflict(channelRecords, input.UserID) {
				return ErrAuthIdentityChannelOwnershipConflict
			}
			if channel == nil {
				channel, err = client.AuthIdentityChannel.Create().
					SetIdentityID(identity.ID).
					SetProviderType(strings.TrimSpace(input.Channel.ProviderType)).
					SetProviderKey(strings.TrimSpace(input.Channel.ProviderKey)).
					SetChannel(strings.TrimSpace(input.Channel.Channel)).
					SetChannelAppID(strings.TrimSpace(input.Channel.ChannelAppID)).
					SetChannelSubject(strings.TrimSpace(input.Channel.ChannelSubject)).
					SetMetadata(copyMetadata(input.ChannelMetadata)).
					Save(txCtx)
				if err != nil {
					return err
				}
			} else {
				targetProviderKey := canonicalizeCompatibleIdentityProviderKey(input.Channel.ProviderType, channel.ProviderKey, input.Channel.ProviderKey)
				update := client.AuthIdentityChannel.UpdateOneID(channel.ID).
					SetIdentityID(identity.ID)
				if targetProviderKey != "" && !strings.EqualFold(targetProviderKey, channel.ProviderKey) {
					update = update.SetProviderKey(targetProviderKey)
				}
				if input.ChannelMetadata != nil {
					update = update.SetMetadata(copyMetadata(input.ChannelMetadata))
				}
				channel, err = update.Save(txCtx)
				if err != nil {
					return err
				}
			}
		}

		result = &CreateAuthIdentityResult{Identity: identity, Channel: channel}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func compatibleIdentityProviderKeys(providerType, providerKey string) []string {
	providerType = strings.TrimSpace(strings.ToLower(providerType))
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" {
		return []string{providerKey}
	}
	if providerType != "wechat" {
		return []string{providerKey}
	}
	keys := []string{providerKey}
	if !strings.EqualFold(providerKey, "wechat-main") {
		keys = append(keys, "wechat-main")
	}
	if !strings.EqualFold(providerKey, "wechat") {
		keys = append(keys, "wechat")
	}
	return keys
}

func canonicalizeCompatibleIdentityProviderKey(providerType, existingKey, requestedKey string) string {
	providerType = strings.TrimSpace(strings.ToLower(providerType))
	existingKey = strings.TrimSpace(existingKey)
	requestedKey = strings.TrimSpace(requestedKey)
	if providerType != "wechat" {
		if requestedKey != "" {
			return requestedKey
		}
		return existingKey
	}
	if strings.EqualFold(existingKey, "wechat") || strings.EqualFold(existingKey, "wechat-main") || strings.EqualFold(requestedKey, "wechat-main") {
		return "wechat-main"
	}
	if requestedKey != "" {
		return requestedKey
	}
	return existingKey
}

func compatibleIdentityProviderKeyRank(providerType, providerKey string) int {
	providerType = strings.TrimSpace(strings.ToLower(providerType))
	providerKey = strings.TrimSpace(providerKey)
	if providerType != "wechat" {
		return 0
	}
	switch {
	case strings.EqualFold(providerKey, "wechat-main"):
		return 0
	case strings.EqualFold(providerKey, "wechat"):
		return 2
	default:
		return 1
	}
}

func selectOwnedCompatibleIdentity(records []*dbent.AuthIdentity, userID int64) *dbent.AuthIdentity {
	var selected *dbent.AuthIdentity
	for _, record := range records {
		if record.UserID != userID {
			continue
		}
		if selected == nil || compatibleIdentityProviderKeyRank(record.ProviderType, record.ProviderKey) < compatibleIdentityProviderKeyRank(selected.ProviderType, selected.ProviderKey) {
			selected = record
		}
	}
	return selected
}

func hasCompatibleIdentityConflict(records []*dbent.AuthIdentity, userID int64) bool {
	for _, record := range records {
		if record.UserID != userID {
			return true
		}
	}
	return false
}

func selectOwnedCompatibleChannel(records []*dbent.AuthIdentityChannel, userID int64) *dbent.AuthIdentityChannel {
	var selected *dbent.AuthIdentityChannel
	for _, record := range records {
		if record.Edges.Identity == nil || record.Edges.Identity.UserID != userID {
			continue
		}
		if selected == nil || compatibleIdentityProviderKeyRank(record.ProviderType, record.ProviderKey) < compatibleIdentityProviderKeyRank(selected.ProviderType, selected.ProviderKey) {
			selected = record
		}
	}
	return selected
}

func hasCompatibleChannelConflict(records []*dbent.AuthIdentityChannel, userID int64) bool {
	for _, record := range records {
		if record.Edges.Identity != nil && record.Edges.Identity.UserID != userID {
			return true
		}
	}
	return false
}

func (r *userRepository) RecordProviderGrant(ctx context.Context, input ProviderGrantRecordInput) (bool, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return false, fmt.Errorf("sql executor is not configured")
	}

	result, err := exec.ExecContext(ctx, `
INSERT INTO user_provider_default_grants (user_id, provider_type, grant_reason)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, provider_type, grant_reason) DO NOTHING`,
		input.UserID,
		strings.TrimSpace(input.ProviderType),
		string(input.GrantReason),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *userRepository) UpsertIdentityAdoptionDecision(ctx context.Context, input IdentityAdoptionDecisionInput) (*dbent.IdentityAdoptionDecision, error) {
	var result *dbent.IdentityAdoptionDecision
	err := r.WithUserProfileIdentityTx(ctx, func(txCtx context.Context) error {
		client := clientFromContext(txCtx, r.client)
		releaseLocks, err := lockRepositoryScopedKeys(
			txCtx,
			client,
			txAwareSQLExecutor(txCtx, r.sql, r.client),
			identityAdoptionDecisionLockKeys(input.PendingAuthSessionID, input.IdentityID)...,
		)
		if err != nil {
			return err
		}
		defer releaseLocks()

		if input.IdentityID != nil && *input.IdentityID > 0 {
			if _, err := client.IdentityAdoptionDecision.Update().
				Where(
					identityadoptiondecision.IdentityIDEQ(*input.IdentityID),
					dbpredicate.IdentityAdoptionDecision(func(s *entsql.Selector) {
						col := s.C(identityadoptiondecision.FieldPendingAuthSessionID)
						s.Where(entsql.Or(
							entsql.IsNull(col),
							entsql.NEQ(col, input.PendingAuthSessionID),
						))
					}),
				).
				ClearIdentityID().
				Save(txCtx); err != nil {
				return err
			}
		}

		create := client.IdentityAdoptionDecision.Create().
			SetPendingAuthSessionID(input.PendingAuthSessionID).
			SetAdoptDisplayName(input.AdoptDisplayName).
			SetAdoptAvatar(input.AdoptAvatar).
			SetDecidedAt(time.Now().UTC())
		if input.IdentityID != nil && *input.IdentityID > 0 {
			create = create.SetIdentityID(*input.IdentityID)
		}

		decisionID, err := create.
			OnConflictColumns(identityadoptiondecision.FieldPendingAuthSessionID).
			UpdateNewValues().
			ID(txCtx)
		if err != nil {
			return err
		}

		result, err = client.IdentityAdoptionDecision.Get(txCtx, decisionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func identityAdoptionDecisionLockKeys(pendingAuthSessionID int64, identityID *int64) []string {
	keys := []string{fmt.Sprintf("identity-adoption:pending:%d", pendingAuthSessionID)}
	if identityID != nil && *identityID > 0 {
		keys = append(keys, fmt.Sprintf("identity-adoption:identity:%d", *identityID))
	}
	return keys
}

func (r *userRepository) GetIdentityAdoptionDecisionByPendingAuthSessionID(ctx context.Context, pendingAuthSessionID int64) (*dbent.IdentityAdoptionDecision, error) {
	return clientFromContext(ctx, r.client).IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(pendingAuthSessionID)).
		Only(ctx)
}

func (r *userRepository) UpdateUserLastLoginAt(ctx context.Context, userID int64, loginAt time.Time) error {
	_, err := clientFromContext(ctx, r.client).User.UpdateOneID(userID).
		SetLastLoginAt(loginAt).
		Save(ctx)
	return err
}

func (r *userRepository) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	_, err := clientFromContext(ctx, r.client).User.UpdateOneID(userID).
		SetLastActiveAt(activeAt).
		Save(ctx)
	return err
}

func (r *userRepository) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	exec, err := r.userProfileIdentitySQL(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := exec.QueryContext(ctx, `
SELECT storage_provider, storage_key, url, content_type, byte_size, sha256
FROM user_avatars
WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return nil, rows.Err()
	}

	var avatar service.UserAvatar
	if err := rows.Scan(
		&avatar.StorageProvider,
		&avatar.StorageKey,
		&avatar.URL,
		&avatar.ContentType,
		&avatar.ByteSize,
		&avatar.SHA256,
	); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &avatar, nil
}

func (r *userRepository) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	exec, err := r.userProfileIdentitySQL(ctx)
	if err != nil {
		return nil, err
	}

	_, err = exec.ExecContext(ctx, `
INSERT INTO user_avatars (user_id, storage_provider, storage_key, url, content_type, byte_size, sha256, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT (user_id) DO UPDATE SET
	storage_provider = EXCLUDED.storage_provider,
	storage_key = EXCLUDED.storage_key,
	url = EXCLUDED.url,
	content_type = EXCLUDED.content_type,
	byte_size = EXCLUDED.byte_size,
	sha256 = EXCLUDED.sha256,
	updated_at = NOW()`,
		userID,
		strings.TrimSpace(input.StorageProvider),
		strings.TrimSpace(input.StorageKey),
		strings.TrimSpace(input.URL),
		strings.TrimSpace(input.ContentType),
		input.ByteSize,
		strings.TrimSpace(input.SHA256),
	)
	if err != nil {
		return nil, err
	}

	return &service.UserAvatar{
		StorageProvider: strings.TrimSpace(input.StorageProvider),
		StorageKey:      strings.TrimSpace(input.StorageKey),
		URL:             strings.TrimSpace(input.URL),
		ContentType:     strings.TrimSpace(input.ContentType),
		ByteSize:        input.ByteSize,
		SHA256:          strings.TrimSpace(input.SHA256),
	}, nil
}

func (r *userRepository) DeleteUserAvatar(ctx context.Context, userID int64) error {
	exec, err := r.userProfileIdentitySQL(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `DELETE FROM user_avatars WHERE user_id = $1`, userID)
	return err
}

func copyMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func validateAuthIdentityChannelProviderMatch(canonical AuthIdentityKey, channel *AuthIdentityChannelKey) error {
	if channel == nil {
		return nil
	}

	canonicalProviderType := strings.TrimSpace(canonical.ProviderType)
	canonicalProviderKey := strings.TrimSpace(canonical.ProviderKey)
	channelProviderType := strings.TrimSpace(channel.ProviderType)
	channelProviderKey := strings.TrimSpace(channel.ProviderKey)

	if canonicalProviderType != channelProviderType || canonicalProviderKey != channelProviderKey {
		return ErrAuthIdentityChannelProviderMismatch
	}

	return nil
}

func txAwareSQLExecutor(ctx context.Context, fallback sqlExecutor, client *dbent.Client) sqlQueryExecutor {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		if exec := sqlExecutorFromEntClient(tx.Client()); exec != nil {
			return exec
		}
	}
	if fallback != nil {
		return fallback
	}
	return sqlExecutorFromEntClient(client)
}

func (r *userRepository) userProfileIdentitySQL(ctx context.Context) (sqlQueryExecutor, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}
	return exec, nil
}

func sqlExecutorFromEntClient(client *dbent.Client) sqlQueryExecutor {
	if client == nil {
		return nil
	}

	clientValue := reflect.ValueOf(client).Elem()
	configValue := clientValue.FieldByName("config")
	driverValue := configValue.FieldByName("driver")
	if !driverValue.IsValid() {
		return nil
	}

	driver := reflect.NewAt(driverValue.Type(), unsafe.Pointer(driverValue.UnsafeAddr())).Elem().Interface()
	exec, ok := driver.(sqlQueryExecutor)
	if !ok {
		return nil
	}
	return exec
}
