//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestScopedAdminPermissionFieldClassification(t *testing.T) {
	for _, tc := range []struct {
		body string
		want []string
	}{
		{`{"extra":{"email":"test@example.com","openai_oauth_responses_websockets_v2_enabled":false,"openai_oauth_responses_websockets_v2_mode":"off","openai_long_context_billing_enabled":false}}`, []string{"accounts.options.write"}},
		{`{"rate_multiplier":0.5}`, []string{"accounts.rate.write"}},
		{`{"credentials":{"expires_at":2000000000,"email":"test@example.com","plan_type":"plus"}}`, []string{"accounts.credentials", "accounts.credentials", "accounts.credentials"}},
	} {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(tc.body), &fields))
		got, err := scopedAccountFieldPermissions(fields, true)
		require.NoError(t, err)
		require.ElementsMatch(t, tc.want, got)
	}
}

func TestScopedAdminPermissionRejectsUnlistedOptions(t *testing.T) {
	for _, body := range []string{`{"extra":{"codex_7d_used_percent":100}}`, `{"extra":{"openai_timezone_mode":"fixed"}}`, `{"extra":{"secret":"x"}}`, `{"extra":{"openai_long_context_billing_enabled":null}}`, `{"extra":{"openai_oauth_responses_websockets_v2_mode":null}}`, `{"rate_multiplier":-1}`, `{"rate_multiplier":null}`} {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(body), &fields))
		_, err := scopedAccountFieldPermissions(fields, true)
		require.Error(t, err, body)
	}
}

type scopedPermissionsCapacityGroups struct{ groupCapacityGroupRepoStub }

func (*scopedPermissionsCapacityGroups) ListScopedAdminCapacityGroupIDs(context.Context) ([]int64, error) {
	return []int64{2}, nil
}
func TestScopedAdminPermissionCapacityScopesBeforeAggregation(t *testing.T) {
	groups := &scopedPermissionsCapacityGroups{groupCapacityGroupRepoStub{groupIDs: []int64{1, 2, 3}}}
	accounts := &groupCapacityAccountRepoStub{}
	svc := NewGroupCapacityService(accounts, groups, nil, nil, nil)
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{})
	result, err := svc.GetAllGroupCapacity(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, accounts.requested)
	require.Equal(t, []GroupCapacitySummary{{GroupID: 2}}, result)
	require.Zero(t, groups.listCalls)
	svc.groupRepo = &groupCapacityGroupRepoStub{}
	_, err = svc.GetAllGroupCapacity(ctx)
	require.Error(t, err)
}

type scopedPermissionCreationRepo struct{ AccountRepository }

func (*scopedPermissionCreationRepo) Create(_ context.Context, a *Account) error {
	a.ID = 1
	return nil
}
func TestScopedAdminPermissionCreateAuthorizesEveryField(t *testing.T) {
	permissions := []string{"accounts.create", "accounts.credentials", "accounts.options.write", "accounts.rate.write", "accounts.schedule"}
	for _, missing := range append([]string{""}, permissions...) {
		t.Run(missing, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			if missing == "" {
				mock.ExpectBegin()
				mock.ExpectCommit()
			}
			svc := &adminServiceImpl{accountRepo: &scopedPermissionCreationRepo{}, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
			granted := []string{}
			for _, permission := range permissions {
				if permission != missing {
					granted = append(granted, permission)
				}
			}
			ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: granted, IncludeUngrouped: true, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}})
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(`{"name":"授权创建","platform":"openai","type":"oauth","group_ids":[],"credentials":{"access_token":"test","refresh_token":"test","expires_at":2000000000,"email":"test@example.com","plan_type":"plus"},"extra":{"email":"test@example.com","openai_oauth_responses_websockets_v2_enabled":false,"openai_oauth_responses_websockets_v2_mode":"off","openai_long_context_billing_enabled":false},"rate_multiplier":0.5,"concurrency":20,"priority":1,"expires_at":null,"auto_pause_on_expired":true}`), &fields))
			a, err := svc.ScopedAdminWrite(ctx, 0, "", fields, nil)
			if missing != "" {
				require.Equal(t, 403, infraerrors.Code(err))
			} else {
				require.NoError(t, err)
				require.Equal(t, 0.5, *a.RateMultiplier)
				require.Equal(t, "test@example.com", a.Credentials["email"])
				require.Equal(t, "test@example.com", a.Extra["email"])
				require.NotContains(t, a.Extra, "privacy_mode")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

type scopedPrivacyCreationRepo struct {
	scopedPermissionCreationRepo
	capturedExtra      map[string]any
	capturedSkip       bool
	capturedPrivacySet bool
}

func (r *scopedPrivacyCreationRepo) Create(ctx context.Context, a *Account) error {
	r.capturedExtra = map[string]any{}
	for key, value := range a.Extra {
		r.capturedExtra[key] = value
	}
	r.capturedSkip = shouldSkipOpenAIPrivacyEnsure(a.Extra)
	r.capturedPrivacySet = a.IsPrivacySet()
	return r.scopedPermissionCreationRepo.Create(ctx, a)
}
func TestScopedAdminPrivacyCompatibilityIgnoresCallerStateBeforeCreate(t *testing.T) {
	for _, mode := range []string{"training_off", "training_disabled", "training_set_failed", "unknown", ""} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectCommit()
			repo := &scopedPrivacyCreationRepo{}
			svc := &adminServiceImpl{accountRepo: repo, entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
			ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials", "accounts.options.write"}, IncludeUngrouped: true, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}})
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(`{"name":"兼容隐私入参","platform":"openai","type":"oauth","group_ids":[],"credentials":{"access_token":"test"}}`), &fields))
			fields["extra"], err = json.Marshal(map[string]any{"privacy_mode": mode, "email": "test@example.com"})
			require.NoError(t, err)
			account, err := svc.ScopedAdminWrite(ctx, 0, "", fields, nil)
			require.NoError(t, err)
			require.Equal(t, "test@example.com", repo.capturedExtra["email"])
			require.NotContains(t, repo.capturedExtra, "privacy_mode", "持久化前必须删除隐私状态键，空字符串同样不能残留")
			require.False(t, repo.capturedSkip, "调用方隐私值不能阻止服务端 Ensure")
			require.False(t, repo.capturedPrivacySet)
			require.NotContains(t, account.Extra, "privacy_mode")
			require.False(t, account.IsPrivacySet())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
func TestScopedAdminPrivacyCompatibilityRejectsInvalidTypes(t *testing.T) {
	for _, value := range []string{`null`, `true`, `1`, `[]`, `{}`, `"` + strings.Repeat("x", 513) + `"`} {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(`{"extra":{"privacy_mode":`+value+`}}`), &fields))
		_, err := scopedAccountFieldPermissions(fields, true)
		require.Equal(t, 400, infraerrors.Code(err), value)
	}
}
