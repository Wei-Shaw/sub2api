package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unsafe"

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
REDACTED

type AuthIdentityChannelKey struct {
	ProviderType   string
	ProviderKey    string
	Channel        string
	ChannelAppID   string
	ChannelSubject string
REDACTED

type CreateAuthIdentityInput struct {
	UserID          int64
	Canonical       AuthIdentityKey
	Channel         *AuthIdentityChannelKey
	Issuer          *string
	VerifiedAt      *time.Time
	Metadata        map[string]any
	ChannelMetadata map[string]any
REDACTED

type BindAuthIdentityInput = CreateAuthIdentityInput

type CreateAuthIdentityResult struct {
	Identity *dbent.AuthIdentity
	Channel  *dbent.AuthIdentityChannel
REDACTED

func (r *CreateAuthIdentityResult) IdentityRef() AuthIdentityKey {
	if r == nil || r.Identity == nil {
		return AuthIdentityKey{REDACTED
REDACTED
	return AuthIdentityKey{
		ProviderType:    r.Identity.ProviderType,
		ProviderKey:     r.Identity.ProviderKey,
		ProviderSubject: r.Identity.ProviderSubject,
REDACTED
REDACTED

func (r *CreateAuthIdentityResult) ChannelRef() *AuthIdentityChannelKey {
	if r == nil || r.Channel == nil {
		return nil
REDACTED
	return &AuthIdentityChannelKey{
		ProviderType:   r.Channel.ProviderType,
		ProviderKey:    r.Channel.ProviderKey,
		Channel:        r.Channel.Channel,
		ChannelAppID:   r.Channel.ChannelAppID,
		ChannelSubject: r.Channel.ChannelSubject,
REDACTED
REDACTED

type UserAuthIdentityLookup struct {
	User     *dbent.User
	Identity *dbent.AuthIdentity
	Channel  *dbent.AuthIdentityChannel
REDACTED

type ProviderGrantRecordInput struct {
	UserID       int64
	ProviderType string
	GrantReason  ProviderGrantReason
REDACTED

type IdentityAdoptionDecisionInput struct {
	PendingAuthSessionID int64
	IdentityID           *int64
	AdoptDisplayName     bool
	AdoptAvatar          bool
REDACTED

type sqlQueryExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
REDACTED

func (r *userRepository) WithUserProfileIdentityTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
REDACTED

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx); err != nil {
		return err
REDACTED
	return tx.Commit()
REDACTED

func (r *userRepository) CreateAuthIdentity(ctx context.Context, input CreateAuthIdentityInput) (*CreateAuthIdentityResult, error) {
	if err := validateAuthIdentityChannelProviderMatch(input.Canonical, input.Channel); err != nil {
		return nil, err
REDACTED

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
REDACTED

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
	REDACTED
REDACTED

	return &CreateAuthIdentityResult{Identity: identity, Channel: channelREDACTED, nil
REDACTED

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
REDACTED

	return &UserAuthIdentityLookup{
		User:     identity.Edges.User,
		Identity: identity,
REDACTED, nil
REDACTED

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
	REDACTED).
		Only(ctx)
	if err != nil {
		return nil, err
REDACTED

	return &UserAuthIdentityLookup{
		User:     channel.Edges.Identity.Edges.User,
		Identity: channel.Edges.Identity,
		Channel:  channel,
REDACTED, nil
REDACTED

func (r *userRepository) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	identities, err := clientFromContext(ctx, r.client).AuthIdentity.Query().
		Where(authidentity.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	records := make([]service.UserAuthIdentityRecord, 0, len(identities))
	for _, identity := range identities {
		if identity == nil {
			continue
	REDACTED
		records = append(records, service.UserAuthIdentityRecord{
			ProviderType:    strings.TrimSpace(identity.ProviderType),
			ProviderKey:     strings.TrimSpace(identity.ProviderKey),
			ProviderSubject: strings.TrimSpace(identity.ProviderSubject),
			VerifiedAt:      identity.VerifiedAt,
			Issuer:          identity.Issuer,
			Metadata:        copyMetadata(identity.Metadata),
			CreatedAt:       identity.CreatedAt,
			UpdatedAt:       identity.UpdatedAt,
	REDACTED)
REDACTED

	return records, nil
REDACTED

func (r *userRepository) BindAuthIdentityToUser(ctx context.Context, input BindAuthIdentityInput) (*CreateAuthIdentityResult, error) {
	if err := validateAuthIdentityChannelProviderMatch(input.Canonical, input.Channel); err != nil {
		return nil, err
REDACTED

	var result *CreateAuthIdentityResult
	err := r.WithUserProfileIdentityTx(ctx, func(txCtx context.Context) error {
		client := clientFromContext(txCtx, r.client)
		canonical := input.Canonical

		identity, err := client.AuthIdentity.Query().
			Where(
				authidentity.ProviderTypeEQ(strings.TrimSpace(canonical.ProviderType)),
				authidentity.ProviderKeyEQ(strings.TrimSpace(canonical.ProviderKey)),
				authidentity.ProviderSubjectEQ(strings.TrimSpace(canonical.ProviderSubject)),
			).
			Only(txCtx)
		if err != nil && !dbent.IsNotFound(err) {
			return err
	REDACTED
		if identity != nil && identity.UserID != input.UserID {
			return ErrAuthIdentityOwnershipConflict
	REDACTED
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
		REDACTED
	REDACTED else {
			update := client.AuthIdentity.UpdateOneID(identity.ID)
			if input.Metadata != nil {
				update = update.SetMetadata(copyMetadata(input.Metadata))
		REDACTED
			if input.Issuer != nil {
				update = update.SetIssuer(strings.TrimSpace(*input.Issuer))
		REDACTED
			if input.VerifiedAt != nil {
				update = update.SetVerifiedAt(*input.VerifiedAt)
		REDACTED
			identity, err = update.Save(txCtx)
			if err != nil {
				return err
		REDACTED
	REDACTED

		var channel *dbent.AuthIdentityChannel
		if input.Channel != nil {
			channel, err = client.AuthIdentityChannel.Query().
				Where(
					authidentitychannel.ProviderTypeEQ(strings.TrimSpace(input.Channel.ProviderType)),
					authidentitychannel.ProviderKeyEQ(strings.TrimSpace(input.Channel.ProviderKey)),
					authidentitychannel.ChannelEQ(strings.TrimSpace(input.Channel.Channel)),
					authidentitychannel.ChannelAppIDEQ(strings.TrimSpace(input.Channel.ChannelAppID)),
					authidentitychannel.ChannelSubjectEQ(strings.TrimSpace(input.Channel.ChannelSubject)),
				).
				WithIdentity().
				Only(txCtx)
			if err != nil && !dbent.IsNotFound(err) {
				return err
		REDACTED
			if channel != nil && channel.Edges.Identity != nil && channel.Edges.Identity.UserID != input.UserID {
				return ErrAuthIdentityChannelOwnershipConflict
		REDACTED
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
			REDACTED
		REDACTED else {
				update := client.AuthIdentityChannel.UpdateOneID(channel.ID).
					SetIdentityID(identity.ID)
				if input.ChannelMetadata != nil {
					update = update.SetMetadata(copyMetadata(input.ChannelMetadata))
			REDACTED
				channel, err = update.Save(txCtx)
				if err != nil {
					return err
			REDACTED
		REDACTED
	REDACTED

		result = &CreateAuthIdentityResult{Identity: identity, Channel: channelREDACTED
		return nil
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	return result, nil
REDACTED

func (r *userRepository) RecordProviderGrant(ctx context.Context, input ProviderGrantRecordInput) (bool, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return false, fmt.Errorf("sql executor is not configured")
REDACTED

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
REDACTED
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
REDACTED
	return affected > 0, nil
REDACTED

func (r *userRepository) UpsertIdentityAdoptionDecision(ctx context.Context, input IdentityAdoptionDecisionInput) (*dbent.IdentityAdoptionDecision, error) {
	client := clientFromContext(ctx, r.client)
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
			REDACTED),
			).
			ClearIdentityID().
			Save(ctx); err != nil {
			return nil, err
	REDACTED
REDACTED

	current, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(input.PendingAuthSessionID)).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
REDACTED
	now := time.Now().UTC()
	if current == nil {
		create := client.IdentityAdoptionDecision.Create().
			SetPendingAuthSessionID(input.PendingAuthSessionID).
			SetAdoptDisplayName(input.AdoptDisplayName).
			SetAdoptAvatar(input.AdoptAvatar).
			SetDecidedAt(now)
		if input.IdentityID != nil {
			create = create.SetIdentityID(*input.IdentityID)
	REDACTED
		return create.Save(ctx)
REDACTED

	update := client.IdentityAdoptionDecision.UpdateOneID(current.ID).
		SetAdoptDisplayName(input.AdoptDisplayName).
		SetAdoptAvatar(input.AdoptAvatar)
	if input.IdentityID != nil {
		update = update.SetIdentityID(*input.IdentityID)
REDACTED
	return update.Save(ctx)
REDACTED

func (r *userRepository) GetIdentityAdoptionDecisionByPendingAuthSessionID(ctx context.Context, pendingAuthSessionID int64) (*dbent.IdentityAdoptionDecision, error) {
	return clientFromContext(ctx, r.client).IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(pendingAuthSessionID)).
		Only(ctx)
REDACTED

func (r *userRepository) UpdateUserLastLoginAt(ctx context.Context, userID int64, loginAt time.Time) error {
	_, err := clientFromContext(ctx, r.client).User.UpdateOneID(userID).
		SetLastLoginAt(loginAt).
		Save(ctx)
	return err
REDACTED

func (r *userRepository) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	_, err := clientFromContext(ctx, r.client).User.UpdateOneID(userID).
		SetLastActiveAt(activeAt).
		Save(ctx)
	return err
REDACTED

func (r *userRepository) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	exec, err := r.userProfileIdentitySQL(ctx)
	if err != nil {
		return nil, err
REDACTED

	rows, err := exec.QueryContext(ctx, `
SELECT storage_provider, storage_key, url, content_type, byte_size, sha256
FROM user_avatars
WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	if !rows.Next() {
		return nil, rows.Err()
REDACTED

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
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return &avatar, nil
REDACTED

func (r *userRepository) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	exec, err := r.userProfileIdentitySQL(ctx)
	if err != nil {
		return nil, err
REDACTED

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
REDACTED

	return &service.UserAvatar{
		StorageProvider: strings.TrimSpace(input.StorageProvider),
		StorageKey:      strings.TrimSpace(input.StorageKey),
		URL:             strings.TrimSpace(input.URL),
		ContentType:     strings.TrimSpace(input.ContentType),
		ByteSize:        input.ByteSize,
		SHA256:          strings.TrimSpace(input.SHA256),
REDACTED, nil
REDACTED

func (r *userRepository) DeleteUserAvatar(ctx context.Context, userID int64) error {
	exec, err := r.userProfileIdentitySQL(ctx)
	if err != nil {
		return err
REDACTED
	_, err = exec.ExecContext(ctx, `DELETE FROM user_avatars WHERE user_id = $1`, userID)
	return err
REDACTED

func (r *userRepository) attachUserAvatar(ctx context.Context, user *service.User) error {
	if user == nil {
		return nil
REDACTED

	avatar, err := r.GetUserAvatar(ctx, user.ID)
	if err != nil {
		return err
REDACTED
	if avatar == nil {
		return nil
REDACTED

	user.AvatarURL = avatar.URL
	user.AvatarSource = avatar.StorageProvider
	user.AvatarMIME = avatar.ContentType
	user.AvatarByteSize = avatar.ByteSize
	user.AvatarSHA256 = avatar.SHA256
	return nil
REDACTED

func copyMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{REDACTED
REDACTED
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
REDACTED
	return out
REDACTED

func validateAuthIdentityChannelProviderMatch(canonical AuthIdentityKey, channel *AuthIdentityChannelKey) error {
	if channel == nil {
		return nil
REDACTED

	canonicalProviderType := strings.TrimSpace(canonical.ProviderType)
	canonicalProviderKey := strings.TrimSpace(canonical.ProviderKey)
	channelProviderType := strings.TrimSpace(channel.ProviderType)
	channelProviderKey := strings.TrimSpace(channel.ProviderKey)

	if canonicalProviderType != channelProviderType || canonicalProviderKey != channelProviderKey {
		return ErrAuthIdentityChannelProviderMismatch
REDACTED

	return nil
REDACTED

func txAwareSQLExecutor(ctx context.Context, fallback sqlExecutor, client *dbent.Client) sqlQueryExecutor {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		if exec := sqlExecutorFromEntClient(tx.Client()); exec != nil {
			return exec
	REDACTED
REDACTED
	if fallback != nil {
		return fallback
REDACTED
	return sqlExecutorFromEntClient(client)
REDACTED

func (r *userRepository) userProfileIdentitySQL(ctx context.Context) (sqlQueryExecutor, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("sql executor is not configured")
REDACTED
	return exec, nil
REDACTED

func sqlExecutorFromEntClient(client *dbent.Client) sqlQueryExecutor {
	if client == nil {
		return nil
REDACTED

	clientValue := reflect.ValueOf(client).Elem()
	configValue := clientValue.FieldByName("config")
	driverValue := configValue.FieldByName("driver")
	if !driverValue.IsValid() {
		return nil
REDACTED

	driver := reflect.NewAt(driverValue.Type(), unsafe.Pointer(driverValue.UnsafeAddr())).Elem().Interface()
	exec, ok := driver.(sqlQueryExecutor)
	if !ok {
		return nil
REDACTED
	return exec
REDACTED
