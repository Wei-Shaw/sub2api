//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedCommitInvalidator struct {
	ids                 []int64
	transactionContexts int
}

func (s *scopedCommitInvalidator) InvalidateToken(ctx context.Context, account *service.Account) error {
	s.ids = append(s.ids, account.ID)
	if dbent.TxFromContext(ctx) != nil {
		s.transactionContexts++
	}
	return nil
}

type scopedCommitAccounts struct {
	service.AdminService
	fail bool
}

func (*scopedCommitAccounts) ScopedAdminList(context.Context, pagination.PaginationParams, string, string, string, string) ([]service.Account, int64, error) {
	panic("不应查询列表")
}
func (s *scopedCommitAccounts) ScopedAdminWrite(_ context.Context, id int64, _ string, _ map[string]json.RawMessage, _ func(context.Context, *service.Account) (*service.Account, error)) (*service.Account, error) {
	if s.fail {
		return nil, errors.New("模拟写入失败")
	}
	return &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}, nil
}

func TestScopedAdminCredentialWritesInvalidateAfterCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"single", "bulk"} {
		for _, mode := range []string{"commit", "rollback", "commit_failure", "write_failure", "name_only"} {
			t.Run(endpoint+"/"+mode, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
				mock.ExpectBegin()
				tx, err := client.Tx(context.Background())
				require.NoError(t, err)
				ctx := dbent.NewTxContext(context.Background(), tx)
				ctx = adminaccess.WithPrincipal(ctx, &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.credentials"}, AllGroups: true}}}})
				cache := &scopedCommitInvalidator{}
				h := &AccountHandler{adminService: &scopedCommitAccounts{fail: mode == "write_failure"}, tokenCacheInvalidator: cache}
				r := gin.New()
				r.Use(func(c *gin.Context) { c.Request = c.Request.WithContext(ctx) })
				r.Use(h.ScopedAdminKeyAccounts)
				r.PUT("/api/v1/admin/accounts/:id", func(*gin.Context) { t.Fatal("不得进入完整管理员处理器") })
				r.POST("/api/v1/admin/accounts/bulk-update", func(*gin.Context) { t.Fatal("不得进入完整管理员处理器") })
				method, path, body := "PUT", "/api/v1/admin/accounts/7", `{"credentials":{"access_token":"fixture"}}`
				if mode == "name_only" {
					body = `{"name":"新名称"}`
				}
				if endpoint == "bulk" {
					method, path = "POST", "/api/v1/admin/accounts/bulk-update"
					body = `{"account_ids":[7],` + body[1:]
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				want := 200
				if mode == "write_failure" && endpoint == "single" {
					want = 503
				}
				require.Equal(t, want, w.Code, w.Body.String())
				require.Empty(t, cache.ids, "事务提交前不得使令牌缓存失效")
				if mode == "rollback" || mode == "write_failure" {
					mock.ExpectRollback()
					require.NoError(t, tx.Rollback())
				} else if mode == "commit_failure" {
					mock.ExpectCommit().WillReturnError(errors.New("提交失败"))
					require.Error(t, tx.Commit())
				} else {
					mock.ExpectCommit()
					require.NoError(t, tx.Commit())
				}
				if mode == "commit" {
					require.Equal(t, []int64{7}, cache.ids)
				} else {
					require.Empty(t, cache.ids)
				}
				require.Zero(t, cache.transactionContexts, "提交后的缓存清理不能携带已关闭事务")
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

type scopedRefreshPrivacyService struct {
	*stubAdminService
	privacy int
}

func (s *scopedRefreshPrivacyService) EnsureOpenAIPrivacy(context.Context, *service.Account) string {
	s.privacy++
	return ""
}
func (s *scopedRefreshPrivacyService) EnsureAntigravityPrivacy(context.Context, *service.Account) string {
	s.privacy++
	return ""
}
func TestScopedAdminRefreshFailureDoesNotTriggerPrivacyBeforeRollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	mock.ExpectBegin()
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	svc := &scopedRefreshPrivacyService{stubAdminService: newStubAdminService()}
	h := &AccountHandler{adminService: svc, openaiOAuthService: service.NewOpenAIOAuthService(nil, nil)}
	_, _, err = h.refreshSingleAccount(dbent.NewTxContext(context.Background(), tx), &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	require.Error(t, err)
	require.Zero(t, svc.privacy, "刷新失败导致回滚时，不应提前调用上游隐私接口")
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.Zero(t, svc.privacy)
	require.NoError(t, mock.ExpectationsWereMet())
	// 完整管理员没有外层事务，沿用原刷新失败后的隐私检查行为。
	_, _, err = h.refreshSingleAccount(context.Background(), &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	require.Error(t, err)
	require.Equal(t, 1, svc.privacy)
}

func TestScopedAdminCodexUpdatesInvalidateOnlyAfterCommit(t *testing.T) {
	for _, mode := range []string{"commit", "rollback", "commit_failure", "identity_changed"} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			mock.ExpectBegin()
			tx, err := client.Tx(context.Background())
			require.NoError(t, err)
			token := buildCodexAccessToken(t, "account", "user", time.Now().Add(time.Hour))
			existing := service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": token, "auth_mode": service.OpenAIAuthModePersonalAccessToken}}
			memory := &scopedCodexMemoryService{codexImportMemoryAdminService: newCodexImportMemoryAdminService([]service.Account{existing})}
			if mode == "identity_changed" {
				memory.fresh = &service.Account{ID: 7, Credentials: map[string]any{"access_token": "changed-private"}}
			}
			cache := &scopedCommitInvalidator{}
			h := &AccountHandler{adminService: memory, tokenCacheInvalidator: cache}
			req := scopedCodexSessionImportRequest{CodexSessionImportRequest: CodexSessionImportRequest{Content: token, GroupIDs: []int64{1}}}
			entries, err := parseCodexSessionImportEntries(req.CodexSessionImportRequest)
			require.NoError(t, err)
			result, err := h.importScopedCodexSessions(dbent.NewTxContext(context.Background(), tx), memory, req, entries)
			require.NoError(t, err)
			require.Empty(t, cache.ids)
			if mode == "rollback" || mode == "identity_changed" {
				mock.ExpectRollback()
				require.NoError(t, tx.Rollback())
			} else if mode == "commit_failure" {
				mock.ExpectCommit().WillReturnError(errors.New("提交失败"))
				require.Error(t, tx.Commit())
			} else {
				mock.ExpectCommit()
				require.NoError(t, tx.Commit())
			}
			if mode == "commit" {
				require.Equal(t, []int64{7}, cache.ids)
			} else {
				require.Empty(t, cache.ids)
			}
			if mode == "identity_changed" {
				require.Equal(t, 1, result.Failed)
			} else {
				require.Equal(t, 1, result.Updated)
				require.Equal(t, service.OpenAIAuthModePersonalAccessToken, memory.updatedAccounts[0].input.Credentials["auth_mode"])
			}
			require.Zero(t, cache.transactionContexts)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
