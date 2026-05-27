# OpenAPI 平台数据集成接口实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 sub2api 后端新增 `/api/v1/openapi/*` 一组 9 个走 `adminAuth` 鉴权的数据集成接口（用户创建/查询、余额调整、LLM key 管理、用量查询），并落地一张 `balance_operations` 幂等表。

**Architecture:** 复用现有 `UserService` / `APIKeyService` / `UsageService` / `userRepository.UpdateBalance`，新增 1 张 Ent schema、1 个 repository、1 个 service、1 个 handler、1 个 routes 文件，并把 handler 注入到 `Handlers` struct + 在 `router.go` 注册路由组。零现有逻辑改动。

**Tech Stack:** Go 1.25.7, Gin, Ent ORM, PostgreSQL, Redis（不直接用）, testify。

**Spec:** `docs/superpowers/specs/2026-05-27-openapi-platform-design.md`

---

## File Structure

新增：
- `backend/ent/schema/balance_operation.go` — Ent schema
- `backend/internal/repository/balance_operation_repo.go` — repo 层
- `backend/internal/repository/balance_operation_repo_integration_test.go` — repo 集成测试
- `backend/internal/service/openapi_service.go` — service 层（编排 user/balance/key/usage）
- `backend/internal/service/openapi_service_test.go` — service 单元测试
- `backend/internal/handler/openapi_handler.go` — gin handlers
- `backend/internal/handler/openapi_handler_test.go` — handler 集成测试
- `backend/internal/server/routes/openapi.go` — 路由注册
- `backend/docs/OPENAPI_PLATFORM.md` 或 `docs/OPENAPI_PLATFORM.md` — 用户向集成指南

修改：
- `backend/internal/handler/handler.go` — `Handlers` struct 加 `OpenAPI *OpenAPIHandler`
- `backend/internal/handler/wire.go`（或 `cmd/server/wire_gen.go`）— 注入构造
- `backend/internal/server/router.go` — 注册 `routes.RegisterOpenAPIRoutes(v1, h, adminAuth)`

---

## Task 1: BalanceOperation Ent Schema + 生成代码

**Files:**
- Create: `backend/ent/schema/balance_operation.go`

- [ ] **Step 1: 写入 schema 文件**

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BalanceOperation 记录通过 openapi 接口对用户余额做的每一次调整，
// 起到幂等键（external_op_id）与审计两个作用。
type BalanceOperation struct {
	ent.Schema
}

func (BalanceOperation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "balance_operations"},
	}
}

func (BalanceOperation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (BalanceOperation) Fields() []ent.Field {
	return []ent.Field{
		field.String("external_op_id").
			MaxLen(128).
			NotEmpty().
			Unique().
			Comment("外部平台传入的操作号，幂等键"),
		field.Int64("user_id").
			Comment("目标用户"),
		field.String("op_type").
			MaxLen(8).
			Comment("set 或 add"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("操作金额"),
		field.Float("balance_before").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("执行前余额"),
		field.Float("balance_after").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("执行后余额"),
		field.String("status").
			MaxLen(16).
			Default("pending").
			Comment("pending / succeeded / failed"),
		field.String("failure_reason").
			MaxLen(255).
			Optional().
			Nillable(),
		field.String("note").
			MaxLen(255).
			Optional().
			Nillable(),
		field.JSON("request_payload", map[string]any{}).
			Optional().
			Comment("入参快照，审计用"),
	}
}

func (BalanceOperation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("status"),
	}
}
```

- [ ] **Step 2: 运行 ent 代码生成**

Run: `cd backend && go generate ./ent`
Expected: 命令成功，无 stderr 报错；`backend/ent/balanceoperation/` 目录与 `backend/ent/balanceoperation.go`、`backend/ent/migrate/schema.go` 中出现 `BalanceOperations` 相关代码。

- [ ] **Step 3: 编译验证**

Run: `cd backend && go build ./...`
Expected: PASS, 无编译错误。

- [ ] **Step 4: Commit**

```bash
git add backend/ent/schema/balance_operation.go backend/ent/
git commit -m "feat(openapi): add balance_operations ent schema for M2M balance audit"
```

---

## Task 2: BalanceOperation Repository

**Files:**
- Create: `backend/internal/repository/balance_operation_repo.go`
- Test: `backend/internal/repository/balance_operation_repo_integration_test.go`

参考样板：`backend/internal/repository/api_key_repo.go` 的构造方式。

- [ ] **Step 1: 在 repo 文件中定义接口与实现**

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/balanceoperation"
)

// BalanceOperation domain 模型——与 ent 实体解耦，避免 service 直接依赖 ent。
type BalanceOperation struct {
	ID             int64
	ExternalOpID   string
	UserID         int64
	OpType         string
	Amount         float64
	BalanceBefore  float64
	BalanceAfter   float64
	Status         string
	FailureReason  *string
	Note           *string
	RequestPayload map[string]any
}

// BalanceOperationRepository repository 接口。
type BalanceOperationRepository interface {
	// FindByExternalOpID 查询幂等记录。找不到返回 (nil, nil)。
	FindByExternalOpID(ctx context.Context, externalOpID string) (*BalanceOperation, error)
	// CreatePending 插入一条 status=pending 的新记录。
	// 当 external_op_id 冲突时返回 ErrDuplicateExternalOpID。
	CreatePending(ctx context.Context, op BalanceOperation) (*BalanceOperation, error)
	// MarkSucceeded 标记成功并写入 balance_before/after。
	MarkSucceeded(ctx context.Context, id int64, balanceBefore, balanceAfter float64) error
	// MarkFailed 标记失败并写入 failure_reason。
	MarkFailed(ctx context.Context, id int64, reason string) error
}

// ErrDuplicateExternalOpID 当 external_op_id 已存在时返回。
var ErrDuplicateExternalOpID = errors.New("duplicate external_op_id")

type balanceOperationRepository struct {
	client *ent.Client
}

// NewBalanceOperationRepository 构造函数。
func NewBalanceOperationRepository(client *ent.Client) BalanceOperationRepository {
	return &balanceOperationRepository{client: client}
}

func (r *balanceOperationRepository) FindByExternalOpID(ctx context.Context, externalOpID string) (*BalanceOperation, error) {
	row, err := r.client.BalanceOperation.Query().
		Where(balanceoperation.ExternalOpIDEQ(externalOpID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("balance op lookup: %w", err)
	}
	return entToBalanceOp(row), nil
}

func (r *balanceOperationRepository) CreatePending(ctx context.Context, op BalanceOperation) (*BalanceOperation, error) {
	create := r.client.BalanceOperation.Create().
		SetExternalOpID(op.ExternalOpID).
		SetUserID(op.UserID).
		SetOpType(op.OpType).
		SetAmount(op.Amount).
		SetStatus("pending")
	if op.Note != nil {
		create.SetNote(*op.Note)
	}
	if op.RequestPayload != nil {
		create.SetRequestPayload(op.RequestPayload)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrDuplicateExternalOpID
		}
		return nil, fmt.Errorf("balance op create: %w", err)
	}
	return entToBalanceOp(row), nil
}

func (r *balanceOperationRepository) MarkSucceeded(ctx context.Context, id int64, balanceBefore, balanceAfter float64) error {
	_, err := r.client.BalanceOperation.UpdateOneID(int(id)).
		SetStatus("succeeded").
		SetBalanceBefore(balanceBefore).
		SetBalanceAfter(balanceAfter).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("balance op mark succeeded: %w", err)
	}
	return nil
}

func (r *balanceOperationRepository) MarkFailed(ctx context.Context, id int64, reason string) error {
	_, err := r.client.BalanceOperation.UpdateOneID(int(id)).
		SetStatus("failed").
		SetFailureReason(reason).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("balance op mark failed: %w", err)
	}
	return nil
}

func entToBalanceOp(row *ent.BalanceOperation) *BalanceOperation {
	op := &BalanceOperation{
		ID:            int64(row.ID),
		ExternalOpID:  row.ExternalOpID,
		UserID:        row.UserID,
		OpType:        row.OpType,
		Amount:        row.Amount,
		BalanceBefore: row.BalanceBefore,
		BalanceAfter:  row.BalanceAfter,
		Status:        row.Status,
	}
	if row.FailureReason != nil {
		op.FailureReason = row.FailureReason
	}
	if row.Note != nil {
		op.Note = row.Note
	}
	if row.RequestPayload != nil {
		op.RequestPayload = row.RequestPayload
	}
	return op
}
```

> 注：ent ID 字段类型默认 `int`；上面 `UpdateOneID(int(id))` 与 schema 兼容。如生成出的 ID 是 `int64`，去掉转换即可。运行编译时会立刻显示。

- [ ] **Step 2: 编译验证**

Run: `cd backend && go build ./internal/repository/...`
Expected: PASS。若 `UpdateOneID(...)` 类型不匹配，按提示调整为 `int` 或 `int64`。

- [ ] **Step 3: 写集成测试**

参考 `backend/internal/repository/api_key_repo_integration_test.go` 的 `// +build integration` 编译标签写法。

```go
//go:build integration
// +build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestBalanceOperationRepository_CreateAndQuery(t *testing.T) {
	client, cleanup := testutil.NewEntClient(t)
	defer cleanup()
	repo := repository.NewBalanceOperationRepository(client)
	ctx := context.Background()

	op := repository.BalanceOperation{
		ExternalOpID: "shop-test-001",
		UserID:       1,
		OpType:       "add",
		Amount:       10.5,
	}
	saved, err := repo.CreatePending(ctx, op)
	require.NoError(t, err)
	require.NotZero(t, saved.ID)
	require.Equal(t, "pending", saved.Status)

	got, err := repo.FindByExternalOpID(ctx, "shop-test-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, saved.ID, got.ID)

	require.NoError(t, repo.MarkSucceeded(ctx, saved.ID, 100.0, 110.5))
	final, err := repo.FindByExternalOpID(ctx, "shop-test-001")
	require.NoError(t, err)
	require.Equal(t, "succeeded", final.Status)
	require.InDelta(t, 110.5, final.BalanceAfter, 1e-6)
}

func TestBalanceOperationRepository_DuplicateRejected(t *testing.T) {
	client, cleanup := testutil.NewEntClient(t)
	defer cleanup()
	repo := repository.NewBalanceOperationRepository(client)
	ctx := context.Background()

	op := repository.BalanceOperation{ExternalOpID: "dup-1", UserID: 1, OpType: "add", Amount: 1}
	_, err := repo.CreatePending(ctx, op)
	require.NoError(t, err)

	_, err = repo.CreatePending(ctx, op)
	require.ErrorIs(t, err, repository.ErrDuplicateExternalOpID)
}
```

> 如果 `internal/testutil.NewEntClient` 不存在，参考 `api_key_repo_integration_test.go` 里使用的实际 helper 名（可能是 `testutil.NewTestEntClient`、`pgtest.NewClient` 等）。先 grep 确认：
> `grep -rn "NewEntClient\|NewTestEntClient\|pgtest" backend/internal/repository/*_integration_test.go | head -5`

- [ ] **Step 4: 跑集成测试**

Run: `cd backend && go test -tags=integration ./internal/repository -run TestBalanceOperationRepository`
Expected: PASS（需要 PostgreSQL 在 127.0.0.1:5432 可达，DSN 通常通过 env）。
失败时检查 testutil helper 是否正确，或修正 schema 类型。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/balance_operation_repo.go backend/internal/repository/balance_operation_repo_integration_test.go
git commit -m "feat(openapi): add BalanceOperationRepository with idempotency support"
```

---

## Task 3: OpenAPIService —— 用户与余额编排逻辑

**Files:**
- Create: `backend/internal/service/openapi_service.go`
- Test: `backend/internal/service/openapi_service_test.go`

参考样板：`backend/internal/service/user_service.go` 的依赖注入风格。

- [ ] **Step 1: 定义 service 结构与构造函数**

```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

// OpenAPIService 编排 openapi 命名空间下的业务动作：
// 创建用户、调整余额、代用户管理 key、查用量。
// 自身不写 ent 客户端，全部委托给已有 service / repo。
type OpenAPIService struct {
	userRepo       UserRepository
	balanceOpRepo  repository.BalanceOperationRepository
	userService    *UserService
	apiKeyService  *APIKeyService
	usageService   *UsageService
}

// NewOpenAPIService 构造函数。
func NewOpenAPIService(
	userRepo UserRepository,
	balanceOpRepo repository.BalanceOperationRepository,
	userService *UserService,
	apiKeyService *APIKeyService,
	usageService *UsageService,
) *OpenAPIService {
	return &OpenAPIService{
		userRepo:      userRepo,
		balanceOpRepo: balanceOpRepo,
		userService:   userService,
		apiKeyService: apiKeyService,
		usageService:  usageService,
	}
}
```

> 注：`UsageService` 名称按 `internal/service/` 实际类型调整。可先 grep：
> `grep -n "type UsageService struct\|type UsageQueryService" backend/internal/service/usage*.go`
> 若实际类型不同（如 `*UsageQueryService`），同步替换。

- [ ] **Step 2: 定义 errors 与 request/response 结构**

```go
// 在 openapi_service.go 中追加：

var (
	ErrOpenAPIEmailRequired    = errors.New("email is required")
	ErrOpenAPIUserNotFound     = errors.New("user not found")
	ErrOpenAPIDuplicateOpID    = errors.New("operation already pending")
	ErrOpenAPIOpFailed         = errors.New("operation previously failed, use a new external_op_id")
	ErrOpenAPIInvalidOpType    = errors.New("op_type must be 'set' or 'add'")
	ErrOpenAPINegativeAmount   = errors.New("amount must be >= 0")
	ErrOpenAPIBalanceUnderflow = errors.New("operation would make balance negative")
)

// CreateUserRequest M2M 创建用户入参。
type CreateUserRequest struct {
	Email           string
	ExternalUserID  string
	InitialBalance  float64
	KeyName         string
	GroupID         *int64
}

// CreateUserResult 创建用户结果——首次成功才包含 APIKey 明文。
type CreateUserResult struct {
	UserID    int64
	Email     string
	Status    string
	Balance   float64
	APIKey    string // 明文，仅首次
	APIKeyID  int64
	FirstTime bool
}

// AdjustBalanceRequest 调整余额入参。
type AdjustBalanceRequest struct {
	ExternalOpID string
	OpType       string  // set 或 add
	Amount       float64
	Note         string
	Payload      map[string]any
}

// AdjustBalanceResult 调整余额结果。
type AdjustBalanceResult struct {
	OperationID      int64
	UserID           int64
	Email            string
	OpType           string
	Amount           float64
	BalanceBefore    float64
	BalanceAfter     float64
	IdempotentReplay bool
}
```

- [ ] **Step 3: 实现 CreateUser**

```go
// 在 openapi_service.go 中追加：

// CreateUser 按 email 创建或返回既有用户。
// - 首次创建：生成随机密码 hash 入库；如带 InitialBalance > 0 直接设余额；可选生成首个 LLM API Key。
// - email 已存在：返回既有 user_id 与当前 balance，不重发任何凭证。
func (s *OpenAPIService) CreateUser(ctx context.Context, req CreateUserRequest) (*CreateUserResult, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return nil, ErrOpenAPIEmailRequired
	}

	if existing, err := s.userRepo.GetByEmail(ctx, email); err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	} else if existing != nil {
		return &CreateUserResult{
			UserID:    existing.ID,
			Email:     existing.Email,
			Status:    existing.Status,
			Balance:   existing.Balance,
			FirstTime: false,
		}, nil
	}

	rawPassword, err := generateRandomPassword(32)
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newUser := &User{
		Email:        email,
		PasswordHash: string(hashed),
		Role:         domain.RoleUser,
		Status:       domain.StatusActive,
		Balance:      req.InitialBalance,
	}
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// 生成首个 LLM API key
	keyName := req.KeyName
	if keyName == "" {
		keyName = "default"
	}
	keyReq := CreateAPIKeyRequest{
		Name:    keyName,
		GroupID: req.GroupID,
	}
	apiKey, err := s.apiKeyService.Create(ctx, newUser.ID, keyReq)
	if err != nil {
		return nil, fmt.Errorf("create initial api key: %w", err)
	}

	return &CreateUserResult{
		UserID:    newUser.ID,
		Email:     newUser.Email,
		Status:    newUser.Status,
		Balance:   newUser.Balance,
		APIKey:    apiKey.Key,
		APIKeyID:  apiKey.ID,
		FirstTime: true,
	}, nil
}

// generateRandomPassword 生成 hex 形式的强随机密码。
func generateRandomPassword(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
```

> 检查点：`UserRepository.GetByEmail / Create` 与 `User` struct 字段（`PasswordHash` / `Balance`）必须与现有 service 定义一致。先 grep `type User struct` 与 `GetByEmail` 签名确认。
> 如 ent password 字段叫别的名（如 `Password` 或 `Hash`），同步替换。

- [ ] **Step 4: 实现 GetUserByEmail**

```go
// GetUserByEmail 按 email 查询用户基础信息。
func (s *OpenAPIService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u, err := s.userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if u == nil {
		return nil, ErrOpenAPIUserNotFound
	}
	return u, nil
}
```

- [ ] **Step 5: 实现 AdjustBalance（含幂等表事务）**

```go
// AdjustBalance 执行 set 或 add 余额调整，幂等键 external_op_id。
func (s *OpenAPIService) AdjustBalance(ctx context.Context, email string, req AdjustBalanceRequest) (*AdjustBalanceResult, error) {
	if req.OpType != "set" && req.OpType != "add" {
		return nil, ErrOpenAPIInvalidOpType
	}
	if req.Amount < 0 {
		return nil, ErrOpenAPINegativeAmount
	}

	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// 幂等检查
	existing, err := s.balanceOpRepo.FindByExternalOpID(ctx, req.ExternalOpID)
	if err != nil {
		return nil, fmt.Errorf("idempotency lookup: %w", err)
	}
	if existing != nil {
		switch existing.Status {
		case "succeeded":
			return &AdjustBalanceResult{
				OperationID:      existing.ID,
				UserID:           existing.UserID,
				Email:            user.Email,
				OpType:           existing.OpType,
				Amount:           existing.Amount,
				BalanceBefore:    existing.BalanceBefore,
				BalanceAfter:     existing.BalanceAfter,
				IdempotentReplay: true,
			}, nil
		case "pending":
			return nil, ErrOpenAPIDuplicateOpID
		case "failed":
			return nil, ErrOpenAPIOpFailed
		}
	}

	note := req.Note
	pending, err := s.balanceOpRepo.CreatePending(ctx, repository.BalanceOperation{
		ExternalOpID:   req.ExternalOpID,
		UserID:         user.ID,
		OpType:         req.OpType,
		Amount:         req.Amount,
		Note:           &note,
		RequestPayload: req.Payload,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateExternalOpID) {
			// 极小概率：刚刚 FindByExternalOpID 没查到但 insert 时被另一并发请求抢先。重读返回。
			latest, _ := s.balanceOpRepo.FindByExternalOpID(ctx, req.ExternalOpID)
			if latest != nil && latest.Status == "succeeded" {
				return &AdjustBalanceResult{
					OperationID:      latest.ID,
					UserID:           latest.UserID,
					Email:            user.Email,
					OpType:           latest.OpType,
					Amount:           latest.Amount,
					BalanceBefore:    latest.BalanceBefore,
					BalanceAfter:     latest.BalanceAfter,
					IdempotentReplay: true,
				}, nil
			}
			return nil, ErrOpenAPIDuplicateOpID
		}
		return nil, fmt.Errorf("create pending op: %w", err)
	}

	// 重新拉最新余额；UserService.UpdateBalance 内部已经做了原子加（参考实现）。
	fresh, err := s.userRepo.GetByID(ctx, user.ID)
	if err != nil {
		_ = s.balanceOpRepo.MarkFailed(ctx, pending.ID, "fetch user balance: "+err.Error())
		return nil, fmt.Errorf("fetch user: %w", err)
	}
	before := fresh.Balance

	var after float64
	switch req.OpType {
	case "set":
		after = req.Amount
	case "add":
		after = before + req.Amount
	}
	if after < 0 {
		_ = s.balanceOpRepo.MarkFailed(ctx, pending.ID, "balance would underflow")
		return nil, ErrOpenAPIBalanceUnderflow
	}

	delta := after - before
	// UserService.UpdateBalance 接受 delta（参考 user_service.go:1058 实现）
	if err := s.userService.UpdateBalance(ctx, user.ID, delta); err != nil {
		_ = s.balanceOpRepo.MarkFailed(ctx, pending.ID, err.Error())
		return nil, fmt.Errorf("update balance: %w", err)
	}

	if err := s.balanceOpRepo.MarkSucceeded(ctx, pending.ID, before, after); err != nil {
		return nil, fmt.Errorf("mark succeeded: %w", err)
	}

	return &AdjustBalanceResult{
		OperationID:      pending.ID,
		UserID:           user.ID,
		Email:            user.Email,
		OpType:           req.OpType,
		Amount:           req.Amount,
		BalanceBefore:    before,
		BalanceAfter:     after,
		IdempotentReplay: false,
	}, nil
}
```

> **重要确认**：`UserService.UpdateBalance(ctx, userID, amount float64)` 实际语义需要先确认是 delta 还是 absolute。读 `internal/service/user_service.go:1058` 看实现：
> ```bash
> sed -n '1050,1085p' backend/internal/service/user_service.go
> ```
> 若是 delta（看到 `Balance = Balance + amount` 或类似），直接传 `delta`。若是 absolute（`Balance = amount`），改成 `s.userService.UpdateBalance(ctx, user.ID, after)`。
> 若 set 与 add 都用增量，那 set 调用同样能正确表达。修正后再 commit。

- [ ] **Step 6: 编译验证**

Run: `cd backend && go build ./internal/service/...`
Expected: PASS。

- [ ] **Step 7: 写单元测试（mock 依赖）**

参考样板：`backend/internal/service/api_key_service_*_test.go` 的 mock 写法。先看现有 mock 的辅助函数命名：

```bash
grep -rn "type fakeUserRepo\|type stubUserRepo\|type mockUserRepository" backend/internal/service/*_test.go | head -5
```

按现有命名风格在 `openapi_service_test.go` 里写：
- `TestCreateUser_NewEmail` — 不存在则建号，返回 FirstTime=true 且 APIKey 非空
- `TestCreateUser_ExistingEmail` — 已存在则 FirstTime=false 且 APIKey 为空
- `TestAdjustBalance_AddSuccess` — add 后余额正确
- `TestAdjustBalance_IdempotentReplay` — 同 external_op_id 第二次调用返回 IdempotentReplay=true 且不二次扣加
- `TestAdjustBalance_FailedReuse` — 已 failed 的 op_id 再调用返回 ErrOpenAPIOpFailed
- `TestAdjustBalance_NegativeUnderflow` — set 后会变成负数返回 ErrOpenAPIBalanceUnderflow

（每个 test 5-15 行；mock 用 testify/mock 或 struct stub，按现有项目风格。）

- [ ] **Step 8: 跑单元测试**

Run: `cd backend && go test -tags=unit ./internal/service -run TestCreateUser -v && go test -tags=unit ./internal/service -run TestAdjustBalance -v`
Expected: 所有用例 PASS。

- [ ] **Step 9: Commit**

```bash
git add backend/internal/service/openapi_service.go backend/internal/service/openapi_service_test.go
git commit -m "feat(openapi): add OpenAPIService with create-user and balance-adjust idempotency"
```

---

## Task 4: OpenAPIService —— Key 与 Usage 编排

**Files:**
- Modify: `backend/internal/service/openapi_service.go`

- [ ] **Step 1: 加 key 相关编排方法（薄封装 APIKeyService）**

```go
// 在 openapi_service.go 末尾追加：

// CreateKeyForEmail 给 email 用户生成一个新 LLM API Key。
func (s *OpenAPIService) CreateKeyForEmail(ctx context.Context, email string, req CreateAPIKeyRequest) (*APIKey, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return s.apiKeyService.Create(ctx, user.ID, req)
}

// ListKeysForEmail 列该 email 用户的 API Keys。
func (s *OpenAPIService) ListKeysForEmail(ctx context.Context, email string, params APIKeyListFilters) ([]APIKey, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	keys, _, err := s.apiKeyService.List(ctx, user.ID, defaultListParams(), params)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// UpdateKeyForEmail 校验归属后改 key。
func (s *OpenAPIService) UpdateKeyForEmail(ctx context.Context, email string, keyID int64, req UpdateAPIKeyRequest) (*APIKey, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return s.apiKeyService.Update(ctx, keyID, user.ID, req)
}

// DeleteKeyForEmail 校验归属后删 key。
func (s *OpenAPIService) DeleteKeyForEmail(ctx context.Context, email string, keyID int64) error {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.apiKeyService.Delete(ctx, keyID, user.ID)
}

func defaultListParams() pagination.PaginationParams {
	return pagination.PaginationParams{
		Page:      1,
		PageSize:  200,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
}
```

> 需要 import `"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"`。
> `APIKeyService.List` 实际签名按 `api_key_handler.go` 用法即 `(ctx, userID, params, filters)`，与上文一致。

- [ ] **Step 2: 加 usage 编排方法（按 user_id 查 usage_log）**

先 grep 现有 usage service 的方法签名：
```bash
grep -n "func (s \*UsageService) List\|func (s \*UsageService) Stats\|GetUsageLogsByUser" backend/internal/service/usage*.go | head -10
```

按发现的签名，写：
```go
// ListUsageForEmail 取该 email 用户的消费明细分页。
func (s *OpenAPIService) ListUsageForEmail(ctx context.Context, email string, params UsageListParams) (*UsageListResult, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	params.UserID = user.ID
	return s.usageService.ListByFilter(ctx, params) // 按现有签名调整
}

// UsageStatsForEmail 取该 email 用户的消费聚合。
func (s *OpenAPIService) UsageStatsForEmail(ctx context.Context, email string, params UsageStatsParams) (*UsageStatsResult, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	params.UserID = user.ID
	return s.usageService.Stats(ctx, params) // 按现有签名调整
}
```

> 如果 `UsageService` 没有现成"按 user 过滤"的方法，先在 `usage_service.go` 加一个最小化的 `ListByUserID(ctx, userID, params)`（直接转交给 `usageRepo.ListByUserID`，repo 一般已有这类方法因为 admin 后台 `/users/:id/usage` 已经支持）。

- [ ] **Step 3: 编译验证**

Run: `cd backend && go build ./internal/service/...`
Expected: PASS。

- [ ] **Step 4: 加 key / usage 编排的单元测试**

- `TestCreateKeyForEmail_UserNotFound` — 不存在 email 时返回 ErrOpenAPIUserNotFound
- `TestListKeysForEmail` — 列出 mock 返回的 keys
- `TestListUsageForEmail_ScopedToUser` — 确保 params.UserID 被正确注入

Run: `cd backend && go test -tags=unit ./internal/service -run "TestCreateKeyForEmail|TestListKeysForEmail|TestListUsageForEmail" -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openapi_service.go backend/internal/service/openapi_service_test.go backend/internal/service/usage_service.go
git commit -m "feat(openapi): add key/usage orchestration in OpenAPIService"
```

---

## Task 5: OpenAPIHandler —— 用户与余额接口

**Files:**
- Create: `backend/internal/handler/openapi_handler.go`

参考样板：`backend/internal/handler/api_key_handler.go` 与 `backend/internal/handler/admin/user_handler.go`。

- [ ] **Step 1: 写 handler 骨架与 user/balance 三个方法**

```go
package handler

import (
	"net/http"
	"net/url"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OpenAPIHandler /api/v1/openapi/* 路由的统一 handler。
// 全部走 adminAuth 中间件鉴权。
type OpenAPIHandler struct {
	svc *service.OpenAPIService
}

// NewOpenAPIHandler 构造函数。
func NewOpenAPIHandler(svc *service.OpenAPIService) *OpenAPIHandler {
	return &OpenAPIHandler{svc: svc}
}

// CreateUserRequest 创建用户入参。
type OpenAPICreateUserRequest struct {
	Email          string  `json:"email" binding:"required,email"`
	ExternalUserID string  `json:"external_user_id"`
	InitialBalance float64 `json:"initial_balance"`
	KeyName        string  `json:"key_name"`
	GroupID        *int64  `json:"group_id"`
}

// CreateUser POST /api/v1/openapi/users
func (h *OpenAPIHandler) CreateUser(c *gin.Context) {
	var req OpenAPICreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	res, err := h.svc.CreateUser(c.Request.Context(), service.CreateUserRequest{
		Email:          req.Email,
		ExternalUserID: req.ExternalUserID,
		InitialBalance: req.InitialBalance,
		KeyName:        req.KeyName,
		GroupID:        req.GroupID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := gin.H{
		"user_id":    res.UserID,
		"email":      res.Email,
		"status":     res.Status,
		"balance":    res.Balance,
		"first_time": res.FirstTime,
	}
	if res.FirstTime {
		out["api_key"] = res.APIKey
		out["api_key_id"] = res.APIKeyID
	}
	response.Success(c, out)
}

// GetUserByEmail GET /api/v1/openapi/users/:email
func (h *OpenAPIHandler) GetUserByEmail(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	user, err := h.svc.GetUserByEmail(c.Request.Context(), email)
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, gin.H{
		"user_id":         user.ID,
		"email":           user.Email,
		"status":          user.Status,
		"role":            user.Role,
		"balance":         user.Balance,
		"total_recharged": user.TotalRecharged,
		"concurrency":     user.Concurrency,
		"created_at":      user.CreatedAt,
	})
}

// AdjustBalanceRequest 调整余额入参。
type OpenAPIAdjustBalanceRequest struct {
	ExternalOpID string  `json:"external_op_id" binding:"required"`
	OpType       string  `json:"op_type" binding:"required,oneof=set add"`
	Amount       float64 `json:"amount" binding:"gte=0"`
	Note         string  `json:"note"`
}

// AdjustBalance PATCH /api/v1/openapi/users/:email/balance
func (h *OpenAPIHandler) AdjustBalance(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req OpenAPIAdjustBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	res, err := h.svc.AdjustBalance(c.Request.Context(), email, service.AdjustBalanceRequest{
		ExternalOpID: req.ExternalOpID,
		OpType:       req.OpType,
		Amount:       req.Amount,
		Note:         req.Note,
		Payload: map[string]any{
			"external_op_id": req.ExternalOpID,
			"op_type":        req.OpType,
			"amount":         req.Amount,
			"note":           req.Note,
		},
	})
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, gin.H{
		"operation_id":      res.OperationID,
		"user_id":           res.UserID,
		"email":             res.Email,
		"op_type":           res.OpType,
		"amount":            res.Amount,
		"balance_before":    res.BalanceBefore,
		"balance_after":     res.BalanceAfter,
		"idempotent_replay": res.IdempotentReplay,
	})
}

// decodeEmailParam 解码 path 里 URL-encoded 的 email。
func decodeEmailParam(c *gin.Context) (string, error) {
	raw := c.Param("email")
	decoded, err := url.PathUnescape(raw)
	if err != nil || decoded == "" {
		return "", &openAPIError{code: http.StatusBadRequest, msg: "invalid email parameter"}
	}
	return decoded, nil
}

type openAPIError struct {
	code int
	msg  string
}

func (e *openAPIError) Error() string { return e.msg }

// mapOpenAPIError 把 service 层错误转成带 HTTP code 的 error。
// 沿用项目里 response.ErrorFrom 的约定（具体如何把 status code 透传依现有 helper 调整）。
func mapOpenAPIError(err error) error {
	switch err {
	case service.ErrOpenAPIUserNotFound:
		return &openAPIError{code: http.StatusNotFound, msg: "user_not_found"}
	case service.ErrOpenAPIDuplicateOpID:
		return &openAPIError{code: http.StatusConflict, msg: "operation_pending"}
	case service.ErrOpenAPIOpFailed:
		return &openAPIError{code: http.StatusConflict, msg: "operation_failed"}
	case service.ErrOpenAPIBalanceUnderflow:
		return &openAPIError{code: http.StatusUnprocessableEntity, msg: "insufficient_balance"}
	case service.ErrOpenAPIInvalidOpType, service.ErrOpenAPINegativeAmount, service.ErrOpenAPIEmailRequired:
		return &openAPIError{code: http.StatusBadRequest, msg: "invalid_request: " + err.Error()}
	default:
		return err
	}
}
```

> **关键确认**：`response.ErrorFrom` 的实际签名决定怎么映射 status code。先 grep：
> ```bash
> grep -n "func ErrorFrom\|func Error\b" backend/internal/pkg/response/*.go
> ```
> 如果它只能接受 `error` 不能透传 code，需要换成 `response.JSON(c, code, ...)` 或在 handler 直接 `c.AbortWithStatusJSON(code, ...)`。修正后再编译。

- [ ] **Step 2: 编译验证**

Run: `cd backend && go build ./internal/handler/...`
Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handler/openapi_handler.go
git commit -m "feat(openapi): add user create/get and balance adjust handlers"
```

---

## Task 6: OpenAPIHandler —— Keys 与 Usage 接口

**Files:**
- Modify: `backend/internal/handler/openapi_handler.go`

- [ ] **Step 1: 加 5 个方法**

```go
// 追加：

// CreateKey POST /api/v1/openapi/users/:email/keys
func (h *OpenAPIHandler) CreateKey(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	svcReq := service.CreateAPIKeyRequest{
		Name:          req.Name,
		GroupID:       req.GroupID,
		CustomKey:     req.CustomKey,
		IPWhitelist:   req.IPWhitelist,
		IPBlacklist:   req.IPBlacklist,
		ExpiresInDays: req.ExpiresInDays,
	}
	if req.Quota != nil {
		svcReq.Quota = *req.Quota
	}
	if req.RateLimit5h != nil {
		svcReq.RateLimit5h = *req.RateLimit5h
	}
	if req.RateLimit1d != nil {
		svcReq.RateLimit1d = *req.RateLimit1d
	}
	if req.RateLimit7d != nil {
		svcReq.RateLimit7d = *req.RateLimit7d
	}
	key, err := h.svc.CreateKeyForEmail(c.Request.Context(), email, svcReq)
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, dto.APIKeyFromService(key))
}

// ListKeys GET /api/v1/openapi/users/:email/keys
func (h *OpenAPIHandler) ListKeys(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filters := service.APIKeyListFilters{
		Status: c.Query("status"),
	}
	keys, err := h.svc.ListKeysForEmail(c.Request.Context(), email, filters)
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Success(c, out)
}

// UpdateKey PATCH /api/v1/openapi/users/:email/keys/:key_id
func (h *OpenAPIHandler) UpdateKey(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	keyID, err := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid key_id")
		return
	}
	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request: "+err.Error())
		return
	}
	// 复用 api_key_handler.Update 里的字段映射代码
	svcReq := buildUpdateAPIKeyServiceRequest(req) // 把 api_key_handler.go 里的映射抽成包级 helper
	updated, err := h.svc.UpdateKeyForEmail(c.Request.Context(), email, keyID, svcReq)
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, dto.APIKeyFromService(updated))
}

// DeleteKey DELETE /api/v1/openapi/users/:email/keys/:key_id
func (h *OpenAPIHandler) DeleteKey(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	keyID, err := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid key_id")
		return
	}
	if err := h.svc.DeleteKeyForEmail(c.Request.Context(), email, keyID); err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// ListUsage GET /api/v1/openapi/users/:email/usage
func (h *OpenAPIHandler) ListUsage(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	params := parseUsageListParams(c) // 按 admin user usage handler 风格抽取
	res, err := h.svc.ListUsageForEmail(c.Request.Context(), email, params)
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, res)
}

// UsageStats GET /api/v1/openapi/users/:email/usage/stats
func (h *OpenAPIHandler) UsageStats(c *gin.Context) {
	email, err := decodeEmailParam(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	params := parseUsageStatsParams(c)
	res, err := h.svc.UsageStatsForEmail(c.Request.Context(), email, params)
	if err != nil {
		response.ErrorFrom(c, mapOpenAPIError(err))
		return
	}
	response.Success(c, res)
}
```

- [ ] **Step 2: 抽出 `buildUpdateAPIKeyServiceRequest` 等 helper**

把 `api_key_handler.go:207-238` 里 `UpdateAPIKeyRequest → service.UpdateAPIKeyRequest` 的映射代码抽到 `api_key_handler.go` 包级私有 helper `buildUpdateAPIKeyServiceRequest(req UpdateAPIKeyRequest) service.UpdateAPIKeyRequest`，原 `Update` 方法改成调用 helper。openapi handler 同样调用 helper，零重复。

类似地，把 admin usage handler 中"解析 query 为 service 参数"的代码抽出 `parseUsageListParams(c) service.UsageListParams` 与 `parseUsageStatsParams(c) service.UsageStatsParams`，放到 `internal/handler/usage_handler.go` 包级私有 helper。

- [ ] **Step 3: 编译验证**

Run: `cd backend && go build ./internal/handler/...`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/
git commit -m "feat(openapi): add keys CRUD and usage handlers; extract shared helpers"
```

---

## Task 7: 注册路由 + 注入 Wire

**Files:**
- Create: `backend/internal/server/routes/openapi.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`（或 `cmd/server/wire_gen.go`）
- Modify: `backend/internal/server/router.go`

- [ ] **Step 1: 写 routes/openapi.go**

```go
package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterOpenAPIRoutes 注册 /api/v1/openapi/* 路由（M2M 数据集成接口）。
// 所有路由套用 adminAuth 中间件。
func RegisterOpenAPIRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	adminAuth middleware.AdminAuthMiddleware,
) {
	openapi := v1.Group("/openapi")
	openapi.Use(gin.HandlerFunc(adminAuth))
	{
		users := openapi.Group("/users")
		{
			users.POST("", h.OpenAPI.CreateUser)
			users.GET("/:email", h.OpenAPI.GetUserByEmail)
			users.PATCH("/:email/balance", h.OpenAPI.AdjustBalance)

			users.POST("/:email/keys", h.OpenAPI.CreateKey)
			users.GET("/:email/keys", h.OpenAPI.ListKeys)
			users.PATCH("/:email/keys/:key_id", h.OpenAPI.UpdateKey)
			users.DELETE("/:email/keys/:key_id", h.OpenAPI.DeleteKey)

			users.GET("/:email/usage", h.OpenAPI.ListUsage)
			users.GET("/:email/usage/stats", h.OpenAPI.UsageStats)
		}
	}
}
```

- [ ] **Step 2: 在 handler.go 加字段**

修改 `backend/internal/handler/handler.go`：在 `Handlers` struct 末尾加：

```go
	OpenAPI          *OpenAPIHandler
```

- [ ] **Step 3: 在 wire / 构造处注入 OpenAPIHandler**

先确认项目用的是手写 wire 还是 google/wire：

```bash
grep -rn "func NewHandlers\|Handlers{\|wire.Build" backend/internal/handler/wire.go backend/cmd/server/wire_gen.go 2>/dev/null | head -15
```

按项目惯例：
1. 找到 `Handlers` 结构体的构造位置（很可能在 `cmd/server/wire_gen.go` 或 `internal/handler/wire.go`）。
2. 在那里 `OpenAPIHandler: handler.NewOpenAPIHandler(openapiService)` 并新增 `openapiService` 局部变量：`openapiService := service.NewOpenAPIService(userRepo, balanceOpRepo, userService, apiKeyService, usageService)`。
3. 确保 `balanceOpRepo := repository.NewBalanceOperationRepository(entClient)` 在调用 NewOpenAPIService 之前构造好。

如果项目用 `google/wire`：修改 `wire.go` 的 provider set 加 `service.NewOpenAPIService`、`handler.NewOpenAPIHandler`、`repository.NewBalanceOperationRepository`，然后 `go generate ./...` 重新生成 `wire_gen.go`。

- [ ] **Step 4: 在 router.go 注册路由**

修改 `backend/internal/server/router.go:112` 之后，加一行：

```go
routes.RegisterOpenAPIRoutes(v1, h, adminAuth)
```

放到 `routes.RegisterAdminRoutes(v1, h, adminAuth)` 之后即可。

- [ ] **Step 5: 编译 + 启动验证**

Run: `cd backend && go build ./cmd/server`
Expected: PASS。

Run: `cd backend && go test -tags=unit ./... -run TestRouter -v` （若有路由测试）
Expected: 不引入回归。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/server/routes/openapi.go backend/internal/handler/handler.go backend/internal/server/router.go backend/internal/handler/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat(openapi): wire OpenAPIHandler into Handlers and register /api/v1/openapi routes"
```

---

## Task 8: 端到端集成测试

**Files:**
- Create: `backend/internal/handler/openapi_handler_test.go`

参考样板：`backend/internal/handler/auth_current_user_test.go` 或 `backend/internal/server/routes/auth_rate_limit_integration_test.go` 的 httptest + gin engine 起服务的写法。

- [ ] **Step 1: 写测试骨架**

```go
//go:build integration
// +build integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/testutil"

	"github.com/stretchr/testify/require"
)

// setupOpenAPIServer 启动一个最小化的 gin engine + 真实 DB + admin api key 已配置。
// 复用项目已有的 testutil 函数（与 admin_basic_handlers_test.go 同款）。
func setupOpenAPIServer(t *testing.T) (string, http.Handler, func()) {
	// TODO: 按现有 testutil pattern 拼装：
	//   - entClient + redisClient
	//   - 构造 services + repos + handlers
	//   - registerOpenAPIRoutes
	//   - 设置 admin api key 到 setting service
	// 返回 (adminAPIKey, handler, cleanup)
	panic("implement using existing test helpers")
}
```

> 第一次写测试时先 grep 项目里其它 handler_test 找 setup helper：
> `grep -rn "func newTestRouter\|func setupAdminRouter\|func setupTestServer" backend/internal/handler/ | head -10`
> 直接复用最接近的样板，必要时给它传入新增的 `OpenAPIHandler`。

- [ ] **Step 2: 写测试用例**

```go
func TestOpenAPI_CreateUser_NewEmail(t *testing.T) {
	adminKey, srv, cleanup := setupOpenAPIServer(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]any{
		"email":           "alice@example.com",
		"initial_balance": 50.0,
	})
	req := httptest.NewRequest("POST", "/api/v1/openapi/users", bytes.NewReader(body))
	req.Header.Set("x-api-key", adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	require.Equal(t, "alice@example.com", data["email"])
	require.Equal(t, true, data["first_time"])
	require.NotEmpty(t, data["api_key"])
}

func TestOpenAPI_CreateUser_ExistingEmail_Idempotent(t *testing.T) {
	// 调两次，验证第二次 first_time=false 且没有 api_key
}

func TestOpenAPI_AdjustBalance_Add(t *testing.T) {
	// 1. 先创建用户
	// 2. PATCH balance op_type=add amount=30 external_op_id="op-1"
	// 3. 验证 balance_after=80
	// 4. 重复同样 external_op_id，验证 idempotent_replay=true 且 balance_after 仍是 80
}

func TestOpenAPI_AdjustBalance_FailedOpIDReuse(t *testing.T) {
	// 直接在 db 里塞一条 status=failed 的 balance_operation，再调用同 op_id 应返回 409
}

func TestOpenAPI_AuthRequired(t *testing.T) {
	// 不带 x-api-key 应返回 401
}

func TestOpenAPI_UserNotFound(t *testing.T) {
	// GET /openapi/users/missing%40example.com 应返回 404
}

func TestOpenAPI_CreateKey_AndList(t *testing.T) {
	// 先建用户，再 POST keys，再 GET keys 验证含两条（首个 + 新建）
}
```

- [ ] **Step 3: 跑集成测试**

Run: `cd backend && go test -tags=integration ./internal/handler -run TestOpenAPI -v`
Expected: 所有用例 PASS。

- [ ] **Step 4: 全量回归**

Run: `cd backend && go test -tags=integration ./...`
Expected: 不引入回归，所有现有集成测试仍 PASS。

Run: `cd backend && go test -tags=unit ./...`
Expected: 所有单元测试 PASS。

Run: `cd backend && golangci-lint run ./...`
Expected: 无新的 lint 错误。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/openapi_handler_test.go
git commit -m "test(openapi): add integration tests covering full openapi flow"
```

---

## Task 9: 文档 + 最终验证

**Files:**
- Create: `docs/OPENAPI_PLATFORM.md`

- [ ] **Step 1: 写用户向集成指南**

内容覆盖：
- 概述：何时用 openapi、与 admin api 的关系、鉴权
- 凭证管理：怎么生成 Admin API Key（指向现有 `/api/v1/admin/settings/admin-api-key` 流程）
- 9 个接口逐个示例（curl）：请求体、成功响应、错误响应
- 幂等性：external_op_id 的使用约定、失败重试策略（换新 op_id）
- 安全性：HTTPS 强制、密码/sk-key 仅首次返回、推荐 IP 白名单（部署时网关侧加）

样例参考已有 `docs/ADMIN_PAYMENT_INTEGRATION_API.md` 的格式。

- [ ] **Step 2: 在 README.md 或 docs/index 加文档索引**

不修改 README 主体，只在最相关章节加一行链接（如果 README 已有"Documentation"小节）；若无则跳过。

- [ ] **Step 3: 手工冒烟测试**

启动本地服务：
```bash
cd backend && go run ./cmd/server
```

用 curl 走一遍（需先在 admin 后台或 settings 表里有一个 admin api key）：
```bash
ADMIN_KEY="sk-admin-..."
BASE="http://localhost:8080/api/v1"

# 1. 创建用户
curl -X POST "$BASE/openapi/users" \
  -H "x-api-key: $ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"email":"smoke@test.local","initial_balance":100}' | jq .

# 2. 查询
curl -H "x-api-key: $ADMIN_KEY" "$BASE/openapi/users/smoke%40test.local" | jq .

# 3. 加余额
curl -X PATCH -H "x-api-key: $ADMIN_KEY" -H "Content-Type: application/json" \
  "$BASE/openapi/users/smoke%40test.local/balance" \
  -d '{"external_op_id":"smoke-1","op_type":"add","amount":30}' | jq .

# 4. 幂等重试
curl -X PATCH -H "x-api-key: $ADMIN_KEY" -H "Content-Type: application/json" \
  "$BASE/openapi/users/smoke%40test.local/balance" \
  -d '{"external_op_id":"smoke-1","op_type":"add","amount":30}' | jq .
# 期望 idempotent_replay=true 且 balance_after 仍是 130

# 5. 查用量
curl -H "x-api-key: $ADMIN_KEY" "$BASE/openapi/users/smoke%40test.local/usage" | jq .
```

如本地没有 admin api key，可暂时通过：
- 启动后访问 admin 后台生成
- 或直接在 settings 表里 INSERT 一行（参考 `internal/service/setting_service.go` 的 key 名）

- [ ] **Step 4: 提交文档**

```bash
git add docs/OPENAPI_PLATFORM.md
git commit -m "docs(openapi): add platform integration API guide"
```

- [ ] **Step 5: 最终全量验证**

```bash
cd backend
go build ./...
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

所有命令 exit 0 即可宣告完成。

---

## Definition of Done

- [x] `balance_operations` 表存在；ent 代码已生成
- [x] 9 个 endpoint 全部可调用
- [x] 创建用户的幂等：同 email 重复调用不重发 sk-key
- [x] 调整余额的幂等：同 external_op_id 不重复扣加
- [x] 失败的 external_op_id 不允许复用
- [x] 所有改动 zero 影响现有路由 / service / 计费
- [x] 单元 + 集成测试 PASS
- [x] golangci-lint 无新错误
- [x] 文档 `docs/OPENAPI_PLATFORM.md` 完成
