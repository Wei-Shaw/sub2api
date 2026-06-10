//go:build integration

// Package routes — support_ticket_integration_test.go
//
// 工单系统端到端 HTTP 集成测试。在真正的 Postgres testcontainer 上跑：
//
//   1. PG 容器启动 + 应用所有 migrations（含 150_add_support_tickets.sql）
//   2. 真实 ent client + SupportTicketRepository + SupportTicketService + handler
//   3. 真实 Gin 路由（用户端走 RegisterSupportRoutes；admin 端按 admin.go 的挂载
//      约定就地组装：在 /api/v1/admin 子组下挂 /support/tickets/...）
//   4. 用一个测试专用的 stub auth middleware 通过 X-Test-User-ID / X-Test-Role
//      头注入登录态——避开 JWT 签发逻辑（已被 jwt_auth 单测覆盖），把焦点放在
//      工单业务流程上。
//
// 之所以放在 routes 包：
//   - handler 包位于 routes 上游，无法在 handler 测试里 import routes 而避免循环依赖
//   - routes 包正好是路由编排的所有权方，集成测试天然属于这里
//   - 与 auth_rate_limit_integration_test.go 同一路径风格
//
// 之所以单独放一个 TestMain：
//   - routes 包此前没有 TestMain；这里加上 build-tag=integration 守卫，对默认/unit
//     编译完全透明，只在 `go test -tags integration ./internal/server/routes/...`
//     时启动 PG 容器
//   - auth_rate_limit_integration_test.go 不依赖 PG（只用 redis），它复用同一
//     TestMain 也无副作用——PG 容器哪怕启动了也不会影响它
//
// 覆盖任务清单（spec §7）：
//   - §7.1 端到端流程：create → user reply → admin reply 切 in_progress →
//     admin patch high → user close → 已关闭再回复 409
//   - §7.2 feature_disabled 场景：用户端 POST /api/v1/support/tickets 返回 404；
//     admin 端 GET /api/v1/admin/support/tickets/:id 仍可访问
//
// 设计权衡：
//   - 用 testify suite + SetupSuite 一次性建容器/迁移；SetupTest 清表 + 重建
//     用户。这比每个测试自己起容器快很多（PG 容器 ~3-5s 启动）
//   - SupportTicketSettingsReader 用 stub 而非 SettingRepo，因为我们要在 §7.2
//     场景里临时把 enabled 翻成 false——直接改 stub 字段比写 settings 表 +
//     清缓存更简单可靠
package routes

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// ---------------------------------------------------------------------------
// TestMain & global PG harness
// ---------------------------------------------------------------------------

// 全局共享，避免每个 test 重新启容器。SetupSuite 通过 routesIntegrationDB /
// routesIntegrationEntClient 拿到这两个值。
var (
	routesIntegrationDB        *sql.DB
	routesIntegrationEntClient *dbent.Client

	// once 保证多个 suite 共享同一个 PG 容器，并保证 TearDownSuite 清理只触发一次。
	routesIntegrationOnce sync.Once
)

const routesIntegrationPGImage = "postgres:18.1-alpine3.23"

func TestMain(m *testing.M) {
	if err := timezone.Init("UTC"); err != nil {
		log.Printf("init timezone: %v", err)
		os.Exit(1)
	}

	// 默认懒启动：只有在 PG 容器真正被 setup 时才创建。这样 auth_rate_limit
	// integration test 单跑时不会因为 PG 容器失败而被拖累。

	code := m.Run()

	// 如果 routesIntegrationOnce 触发过，关闭资源。
	if routesIntegrationEntClient != nil {
		_ = routesIntegrationEntClient.Close()
	}
	if routesIntegrationDB != nil {
		_ = routesIntegrationDB.Close()
	}

	os.Exit(code)
}

// setupRoutesPG 懒加载 PG 容器 + 应用迁移 + 构建 ent client。第二次及之后调用
// 直接返回已构建的全局值。
func setupRoutesPG(t *testing.T) (*sql.DB, *dbent.Client) {
	t.Helper()

	routesIntegrationOnce.Do(func() {
		if !routesDockerAvailable() {
			t.Skip("Docker 未启用，跳过工单系统集成测试")
			return
		}

		ctx := context.Background()
		container, err := tcpostgres.Run(
			ctx,
			routesIntegrationPGImage,
			tcpostgres.WithDatabase("sub2api_routes_test"),
			tcpostgres.WithUsername("postgres"),
			tcpostgres.WithPassword("postgres"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			t.Fatalf("start postgres container: %v", err)
		}
		// 容器在 TestMain 末尾不显式 Terminate（让 testcontainers reaper 处理），
		// 这与 repository/integration_harness_test.go 保持类似简化策略。

		dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
		if err != nil {
			t.Fatalf("get pg dsn: %v", err)
		}

		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatalf("open pg: %v", err)
		}

		// 等 PG 真正就绪
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if err := db.PingContext(ctx); err == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}

		// 应用所有 SQL 迁移（含我们的 150_add_support_tickets.sql）
		if err := repository.ApplyMigrations(ctx, db); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}

		drv := entsql.OpenDB(dialect.Postgres, db)
		client := dbent.NewClient(dbent.Driver(drv))

		routesIntegrationDB = db
		routesIntegrationEntClient = client
	})

	if routesIntegrationDB == nil || routesIntegrationEntClient == nil {
		t.Skip("PG 容器未就绪，跳过")
	}
	return routesIntegrationDB, routesIntegrationEntClient
}

func routesDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

// ---------------------------------------------------------------------------
// Stub Settings Reader
// ---------------------------------------------------------------------------

// stubSupportSettingsReader 实现 service.SupportTicketSettingsReader。
// 所有字段都用同步原语保护，避免并行测试串号（虽然测试基本上是串行的）。
type stubSupportSettingsReader struct {
	mu              sync.RWMutex
	enabled         bool
	categories      []string
	defaultPriority string
}

func newStubSupportSettings(enabled bool) *stubSupportSettingsReader {
	return &stubSupportSettingsReader{
		enabled:         enabled,
		categories:      []string{"充值", "账号", "API", "Bug", "其他"},
		defaultPriority: service.SupportTicketPriorityNormal,
	}
}

func (s *stubSupportSettingsReader) GetSupportTicketRuntime(context.Context) service.SupportTicketRuntime {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cats := make([]string, len(s.categories))
	copy(cats, s.categories)
	return service.SupportTicketRuntime{
		Enabled:         s.enabled,
		Categories:      cats,
		DefaultPriority: s.defaultPriority,
	}
}

func (s *stubSupportSettingsReader) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

// SupportTicketIntegrationSuite 是工单系统的 HTTP 端到端集成测试套件。
type SupportTicketIntegrationSuite struct {
	suite.Suite

	db        *sql.DB
	entClient *dbent.Client

	settings *stubSupportSettingsReader
	router   *gin.Engine

	// 每个测试 SetupTest 时新建：
	user1ID int64
	user2ID int64
	adminID int64
}

func TestSupportTicketIntegrationSuite(t *testing.T) {
	if !routesDockerAvailable() {
		t.Skip("Docker 未启用，跳过工单系统集成测试")
	}
	suite.Run(t, new(SupportTicketIntegrationSuite))
}

func (s *SupportTicketIntegrationSuite) SetupSuite() {
	db, client := setupRoutesPG(s.T())
	s.db = db
	s.entClient = client
}

func (s *SupportTicketIntegrationSuite) SetupTest() {
	// 清表（按 FK 反向顺序）。CASCADE 已经能处理 ON DELETE CASCADE，
	// 但 users 上挂了很多旁支表，这里只清和工单相关的表 + 我们新建的用户。
	ctx := context.Background()
	for _, stmt := range []string{
		`TRUNCATE TABLE support_ticket_replies RESTART IDENTITY CASCADE`,
		`TRUNCATE TABLE support_tickets RESTART IDENTITY CASCADE`,
	} {
		_, err := s.db.ExecContext(ctx, stmt)
		s.Require().NoError(err, "truncate: %s", stmt)
	}

	// 只清这次创建的测试用户，避免影响 atlas baseline / 系统 setting 行
	if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE email LIKE 'support-it-%'`); err != nil {
		s.Require().NoError(err)
	}

	// 创建 3 个测试用户：user1（owner）、user2（不相关用户）、admin。
	s.user1ID = s.createUser("support-it-user1@example.com", "user")
	s.user2ID = s.createUser("support-it-user2@example.com", "user")
	s.adminID = s.createUser("support-it-admin@example.com", "admin")

	// 重新构造 settings stub + router（避免上一个测试改了 enabled 影响下一个）
	s.settings = newStubSupportSettings(true)
	s.router = s.buildRouter()
}

// createUser 直接用 ent 建用户。绕开 user_repo 的语义校验，加快测试。
func (s *SupportTicketIntegrationSuite) createUser(email, role string) int64 {
	u, err := s.entClient.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetRole(role).
		Save(context.Background())
	s.Require().NoError(err, "create user %s", email)
	return u.ID
}

// buildRouter 组装一个最小可工作的 gin 路由：
//   - 用户端：/api/v1/support/* 走 RegisterSupportRoutes 真实路径
//   - admin 端：内联 admin handler 路由，避免 RegisterAdminRoutes 的全套依赖
//
// 用一个测试 stub auth middleware 通过 X-Test-User-ID / X-Test-Role 头注入登录态。
func (s *SupportTicketIntegrationSuite) buildRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 真实的 SupportTicket service stack
	stRepo := repository.NewSupportTicketRepository(s.entClient)
	stSvc := service.NewSupportTicketService(stRepo, s.settings, s.entClient)
	stHandler := handler.NewSupportTicketHandler(stSvc)
	adminStHandler := admin.NewSupportTicketHandler(stSvc)

	// stub auth middleware：从 header 解析 X-Test-User-ID / X-Test-Role，注入
	// AuthSubject 与角色到 gin context，与项目 jwt_auth 中间件的语义对齐。
	authStub := func(c *gin.Context) {
		raw := c.GetHeader("X-Test-User-ID")
		if raw == "" {
			c.Next()
			return
		}
		uid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || uid <= 0 {
			c.Next()
			return
		}
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{
			UserID:      uid,
			Concurrency: 1,
		})
		role := c.GetHeader("X-Test-Role")
		if role == "" {
			role = "user"
		}
		c.Set(string(servermiddleware.ContextKeyUserRole), role)
		c.Next()
	}

	v1 := router.Group("/api/v1")

	// 用户端：复用真实 RegisterSupportRoutes（走 BackendModeUserGuard，settingService=nil
	// 时 guard 直接放行，不影响测试）
	RegisterSupportRoutes(
		v1,
		&handler.Handlers{
			SupportTicket: stHandler,
		},
		servermiddleware.JWTAuthMiddleware(authStub),
		// optionalJWTAuth：本测试不覆盖 chat 路由，挂个 noop 即可；不能传 nil
		// （RegisterSupportRoutes 会 chat.Use(gin.HandlerFunc(optionalJWTAuth))，
		// gin 在挂 nil handler 时会 panic）。
		servermiddleware.OptionalJWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil, // settingService=nil → BackendModeUserGuard 直接放行
		nil, // rateLimiter=nil → support_chat 子组只挂路由不挂限流（本测试不覆盖 chat 路由）
	)

	// admin 端：内联挂载（避开 RegisterAdminRoutes 的全套依赖）。这与 routes/admin.go
	// 内 registerAdminSupportRoutes 的路径完全一致，只是省去 admin 父组的
	// AdminAuthMiddleware（用 authStub 替代）。
	adminGroup := v1.Group("/admin", authStub)
	{
		support := adminGroup.Group("/support")
		tickets := support.Group("/tickets")
		tickets.GET("", adminStHandler.List)
		tickets.GET("/:id", adminStHandler.Get)
		tickets.POST("/:id/replies", adminStHandler.AppendReply)
		tickets.PATCH("/:id", adminStHandler.Patch)
	}

	return router
}

// ---------------------------------------------------------------------------
// HTTP 辅助
// ---------------------------------------------------------------------------

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Reason  string          `json:"reason,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// doJSON 发起一次 JSON 请求并解码外层 envelope。
func (s *SupportTicketIntegrationSuite) doJSON(method, path string, userID int64, role string, body any) (int, apiEnvelope) {
	var buf bytes.Buffer
	if body != nil {
		s.Require().NoError(json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID > 0 {
		req.Header.Set("X-Test-User-ID", strconv.FormatInt(userID, 10))
	}
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var env apiEnvelope
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &env)
	}
	return w.Code, env
}

// decodeData 把 envelope.Data 反序列化到 out。
func (s *SupportTicketIntegrationSuite) decodeData(env apiEnvelope, out any) {
	s.Require().NotEmpty(env.Data, "envelope.data should not be empty")
	s.Require().NoError(json.Unmarshal(env.Data, out))
}

// ---------------------------------------------------------------------------
// §7.1 端到端流程
// ---------------------------------------------------------------------------

// TestEndToEndFlow 覆盖完整工单生命周期：
//
//   1. user1 创建工单（status=open, 默认 normal）
//   2. user1 列表能看到自己工单（不含 chat_context）
//   3. user2 GET 同一工单 → 404（owner 隔离）
//   4. user1 追加回复（status 仍为 open，user 回复不触发跃迁）
//   5. admin 回复 → status 自动跃迁 open→in_progress
//   6. admin PATCH priority=high
//   7. user1 关闭工单（status=closed, closed_at 非空）
//   8. user1 在已关闭工单上再追加回复 → 409
//   9. admin PATCH 试图把 closed 改回 open → 409
func (s *SupportTicketIntegrationSuite) TestEndToEndFlow() {
	chatCtx := "user: 我充值后没到账\nassistant: 已记录"

	// 1. user1 创建工单
	status, env := s.doJSON(http.MethodPost, "/api/v1/support/tickets", s.user1ID, "user", map[string]any{
		"title":        "充值未到账",
		"content":      "我昨天充值 100 没到账",
		"category":     "充值",
		"chat_context": chatCtx,
	})
	s.Require().Equal(http.StatusOK, status, "create: %s", env.Message)

	type detailDTO struct {
		ID          int64   `json:"id"`
		UserID      int64   `json:"user_id"`
		Title       string  `json:"title"`
		Status      string  `json:"status"`
		Priority    string  `json:"priority"`
		Category    string  `json:"category"`
		ChatContext *string `json:"chat_context,omitempty"`
		Replies     []struct {
			ID       int64  `json:"id"`
			TicketID int64  `json:"ticket_id"`
			IsAdmin  bool   `json:"is_admin"`
			Content  string `json:"content"`
		} `json:"replies"`
		ClosedAt *time.Time `json:"closed_at,omitempty"`
	}
	var created detailDTO
	s.decodeData(env, &created)
	s.Require().NotZero(created.ID)
	s.Require().Equal(s.user1ID, created.UserID)
	s.Require().Equal(service.SupportTicketStatusOpen, created.Status)
	s.Require().Equal(service.SupportTicketPriorityNormal, created.Priority, "default priority")
	s.Require().NotNil(created.ChatContext, "create response should include chat_context")
	s.Require().Equal(chatCtx, *created.ChatContext)
	s.Require().Empty(created.Replies, "new ticket should have empty replies")
	ticketID := created.ID

	// 2. user1 列表能看到工单，且 chat_context 字段不存在
	status, env = s.doJSON(http.MethodGet, "/api/v1/support/tickets", s.user1ID, "user", nil)
	s.Require().Equal(http.StatusOK, status)
	type listResp struct {
		Items []json.RawMessage `json:"items"`
		Total int64             `json:"total"`
	}
	var lr listResp
	s.decodeData(env, &lr)
	s.Require().Equal(int64(1), lr.Total)
	s.Require().Len(lr.Items, 1)
	// 列表项里不应有 chat_context 键（DTO 编译期不含该字段）
	var listItemMap map[string]any
	s.Require().NoError(json.Unmarshal(lr.Items[0], &listItemMap))
	_, hasChat := listItemMap["chat_context"]
	s.Require().False(hasChat, "list view must not expose chat_context")

	// 3. user2 GET 同一工单 → 404（owner 隔离，避免泄露存在性）
	status, _ = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/support/tickets/%d", ticketID), s.user2ID, "user", nil)
	s.Require().Equal(http.StatusNotFound, status, "non-owner should see 404")

	// 4. user1 追加回复，status 仍为 open
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/support/tickets/%d/replies", ticketID), s.user1ID, "user", map[string]any{
		"content": "请帮忙看一下",
	})
	s.Require().Equal(http.StatusOK, status)

	status, env = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/support/tickets/%d", ticketID), s.user1ID, "user", nil)
	s.Require().Equal(http.StatusOK, status)
	var afterUserReply detailDTO
	s.decodeData(env, &afterUserReply)
	s.Require().Equal(service.SupportTicketStatusOpen, afterUserReply.Status, "user reply must not transition status")
	s.Require().Len(afterUserReply.Replies, 1)
	s.Require().False(afterUserReply.Replies[0].IsAdmin)

	// 5. admin 回复 → status 自动 open → in_progress
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/admin/support/tickets/%d/replies", ticketID), s.adminID, "admin", map[string]any{
		"content": "已收到，正在排查",
	})
	s.Require().Equal(http.StatusOK, status)

	status, env = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/admin/support/tickets/%d", ticketID), s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status)
	var afterAdminReply detailDTO
	s.decodeData(env, &afterAdminReply)
	s.Require().Equal(service.SupportTicketStatusInProgress, afterAdminReply.Status, "admin reply triggers open→in_progress")
	s.Require().Len(afterAdminReply.Replies, 2)
	s.Require().True(afterAdminReply.Replies[1].IsAdmin)

	// 6. admin PATCH priority=high
	high := service.SupportTicketPriorityHigh
	status, _ = s.doJSON(http.MethodPatch, fmt.Sprintf("/api/v1/admin/support/tickets/%d", ticketID), s.adminID, "admin", map[string]any{
		"priority": high,
	})
	s.Require().Equal(http.StatusOK, status)

	status, env = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/admin/support/tickets/%d", ticketID), s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status)
	var afterPatch detailDTO
	s.decodeData(env, &afterPatch)
	s.Require().Equal(high, afterPatch.Priority)
	// status 不应被 PATCH 影响
	s.Require().Equal(service.SupportTicketStatusInProgress, afterPatch.Status)

	// 7. user1 关闭工单
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/support/tickets/%d/close", ticketID), s.user1ID, "user", nil)
	s.Require().Equal(http.StatusOK, status)

	status, env = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/support/tickets/%d", ticketID), s.user1ID, "user", nil)
	s.Require().Equal(http.StatusOK, status)
	var afterClose detailDTO
	s.decodeData(env, &afterClose)
	s.Require().Equal(service.SupportTicketStatusClosed, afterClose.Status)
	s.Require().NotNil(afterClose.ClosedAt, "closed_at should be set")

	// 8. user1 在已关闭工单上再回复 → 409
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/support/tickets/%d/replies", ticketID), s.user1ID, "user", map[string]any{
		"content": "再问一句",
	})
	s.Require().Equal(http.StatusConflict, status, "reply on closed ticket must be 409")

	// 9. admin 试图 reopen closed 工单 → 409
	openStatus := service.SupportTicketStatusOpen
	status, _ = s.doJSON(http.MethodPatch, fmt.Sprintf("/api/v1/admin/support/tickets/%d", ticketID), s.adminID, "admin", map[string]any{
		"status": openStatus,
	})
	s.Require().Equal(http.StatusConflict, status, "reopen closed must be 409")
}

// TestEndToEndFlow_AdminListFilters 验证 admin 列表 + 过滤 + 排序在 PG 上真正生效。
func (s *SupportTicketIntegrationSuite) TestEndToEndFlow_AdminListFilters() {
	// 准备 3 张工单：高/中/低优先级，分类各异
	create := func(uid int64, title, category, priority string) int64 {
		_, env := s.doJSON(http.MethodPost, "/api/v1/support/tickets", uid, "user", map[string]any{
			"title":    title,
			"content":  "content of " + title,
			"category": category,
			"priority": priority,
		})
		var d struct {
			ID int64 `json:"id"`
		}
		s.decodeData(env, &d)
		return d.ID
	}

	idHigh := create(s.user1ID, "P-HIGH", "API", service.SupportTicketPriorityHigh)
	idNormal := create(s.user1ID, "P-NORMAL", "Bug", service.SupportTicketPriorityNormal)
	idLow := create(s.user2ID, "P-LOW", "充值", service.SupportTicketPriorityLow)

	// admin list 默认排序 priority CASE-DESC：high → normal → low
	status, env := s.doJSON(http.MethodGet, "/api/v1/admin/support/tickets?page=1&page_size=10", s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status)
	var lr struct {
		Items []struct {
			ID       int64  `json:"id"`
			Priority string `json:"priority"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	s.decodeData(env, &lr)
	s.Require().GreaterOrEqual(lr.Total, int64(3))
	s.Require().Equal(idHigh, lr.Items[0].ID)
	s.Require().Equal(idNormal, lr.Items[1].ID)
	s.Require().Equal(idLow, lr.Items[2].ID)

	// 过滤 category=API 只返回 1 条
	status, env = s.doJSON(http.MethodGet, "/api/v1/admin/support/tickets?category=API", s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status)
	s.decodeData(env, &lr)
	s.Require().Equal(int64(1), lr.Total)
	s.Require().Equal(idHigh, lr.Items[0].ID)

	// 过滤 user_id=user2ID 只返回 user2 的低优先级
	status, env = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/admin/support/tickets?user_id=%d", s.user2ID), s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status)
	s.decodeData(env, &lr)
	s.Require().Equal(int64(1), lr.Total)
	s.Require().Equal(idLow, lr.Items[0].ID)

	// q 关键词命中 title
	status, env = s.doJSON(http.MethodGet, "/api/v1/admin/support/tickets?q=P-HIGH", s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status)
	s.decodeData(env, &lr)
	s.Require().Equal(int64(1), lr.Total)
	s.Require().Equal(idHigh, lr.Items[0].ID)

	// 非法 user_id 返回 400
	status, _ = s.doJSON(http.MethodGet, "/api/v1/admin/support/tickets?user_id=abc", s.adminID, "admin", nil)
	s.Require().Equal(http.StatusBadRequest, status)
}

// ---------------------------------------------------------------------------
// §7.2 feature_disabled 场景
// ---------------------------------------------------------------------------

// TestFeatureDisabled_UserBlockedAdminAccessible 覆盖 spec §7.2：
//
//   - feature_disabled = true 时 POST /api/v1/support/tickets 返回 404
//   - GET /api/v1/support/categories 返回 404
//   - admin 路由（List / Get / Patch / AppendReply）仍然可访问，方便管理员
//     提前编辑/收尾历史工单
func (s *SupportTicketIntegrationSuite) TestFeatureDisabled_UserBlockedAdminAccessible() {
	// 先用 enabled=true 创一张工单，模拟"曾经开过工单系统"的存量状态。
	status, env := s.doJSON(http.MethodPost, "/api/v1/support/tickets", s.user1ID, "user", map[string]any{
		"title":    "存量工单",
		"content":  "feature 关闭前创建",
		"category": "其他",
	})
	s.Require().Equal(http.StatusOK, status)
	var created struct {
		ID int64 `json:"id"`
	}
	s.decodeData(env, &created)
	ticketID := created.ID

	// 关闭 feature flag
	s.settings.SetEnabled(false)

	// 1. user 端 POST /tickets → 404
	status, _ = s.doJSON(http.MethodPost, "/api/v1/support/tickets", s.user1ID, "user", map[string]any{
		"title": "x", "content": "y", "category": "其他",
	})
	s.Require().Equal(http.StatusNotFound, status, "feature disabled: POST tickets must 404")

	// 2. user 端 GET /categories → 404
	status, _ = s.doJSON(http.MethodGet, "/api/v1/support/categories", s.user1ID, "user", nil)
	s.Require().Equal(http.StatusNotFound, status, "feature disabled: categories must 404")

	// 3. user 端 GET /tickets/:id → 404（service 层 enabled 守卫）
	status, _ = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/support/tickets/%d", ticketID), s.user1ID, "user", nil)
	s.Require().Equal(http.StatusNotFound, status, "feature disabled: GET ticket must 404")

	// 4. user 端追加回复 → 404
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/support/tickets/%d/replies", ticketID), s.user1ID, "user", map[string]any{
		"content": "test",
	})
	s.Require().Equal(http.StatusNotFound, status, "feature disabled: user reply must 404")

	// 5. user 端 close → 404
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/support/tickets/%d/close", ticketID), s.user1ID, "user", nil)
	s.Require().Equal(http.StatusNotFound, status, "feature disabled: user close must 404")

	// 6. admin 端 GET /tickets/:id 仍然可访问
	status, _ = s.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/admin/support/tickets/%d", ticketID), s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status, "admin route must remain accessible when feature disabled")

	// 7. admin 端 List 仍然可访问
	status, _ = s.doJSON(http.MethodGet, "/api/v1/admin/support/tickets", s.adminID, "admin", nil)
	s.Require().Equal(http.StatusOK, status, "admin list must remain accessible")

	// 8. admin 端 AppendReply 仍然可访问（不卡 enabled，spec 7.2）
	status, _ = s.doJSON(http.MethodPost, fmt.Sprintf("/api/v1/admin/support/tickets/%d/replies", ticketID), s.adminID, "admin", map[string]any{
		"content": "我们看到了",
	})
	s.Require().Equal(http.StatusOK, status, "admin reply must work when feature disabled")

	// 9. admin 端 PATCH 仍然可访问
	high := service.SupportTicketPriorityHigh
	status, _ = s.doJSON(http.MethodPatch, fmt.Sprintf("/api/v1/admin/support/tickets/%d", ticketID), s.adminID, "admin", map[string]any{
		"priority": high,
	})
	s.Require().Equal(http.StatusOK, status, "admin patch must work when feature disabled")
}

// ---------------------------------------------------------------------------
// 兜底：基本鉴权
// ---------------------------------------------------------------------------

// TestUnauthenticatedReturns401 确保即使在 PG 真实栈上，缺鉴权头时也能落到
// 401（handler 层 GetAuthSubjectFromContext 守卫）。
func (s *SupportTicketIntegrationSuite) TestUnauthenticatedReturns401() {
	require := require.New(s.T())

	// 不设 X-Test-User-ID
	status, _ := s.doJSON(http.MethodPost, "/api/v1/support/tickets", 0, "", map[string]any{
		"title":    "x",
		"content":  "y",
		"category": "其他",
	})
	require.Equal(http.StatusUnauthorized, status)
}
