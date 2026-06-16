package admin

// 通用 admin handler 辅助函数：URL path 参数解析、JSON 请求体绑定。
//
// 动机：admin 包下 40+ handler 各自重复写
//   - strconv.ParseInt(c.Param("id"), 10, 64) + 自由文案的 BadRequest
//   - c.ShouldBindJSON(&req) + 自由文案的 BadRequest
// 按 CLAUDE.md §4 API 错误应返回结构化（code + message + metadata），由前端做 i18n。
// 现在先把公共片段抽到 admin 包级别，quota_handler 先行迁移；其余 handler 在
// 后续 PR 里逐步替换私有实现，避免一次性触及大面积 diff。

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// admin 包内多个 handler 共享的通用错误码常量。
//
// 业务专属的 reason（如 INVALID_USER_ID / TURNSTILE_SITE_KEY_REQUIRED 等）放在各自
// handler 顶部 const 块；这里只放跨 handler 一致语义的高频 reason，避免散落多份定义后漂移。
//
// 与前端 i18n 表对齐：common.errors.INVALID_REQUEST_BODY / common.errors.INVALID_ID。
const (
	errReasonInvalidRequestBody = "INVALID_REQUEST_BODY"
	errReasonInvalidID          = "INVALID_ID"
)

// ParseInt64Param 解析 gin path 参数为 int64，解析失败返回字段级 ValidationFailed。
//
// invalidCode 是顶层 ApplicationError.Reason（例如 INVALID_ID），与前端 i18n 表对齐；
// 同时返回 fields=[{path: name, code: INVALID_VALUE}] 让前端能像表单字段错一样行内渲染——
// 让"路径参数错"与"body 字段错"在前端走同一份 parseFieldErrors，无需按 reason 分支。
//
// 值范围校验（>0、上限等）留给 service 层；本函数只承担类型转换。
func ParseInt64Param(c *gin.Context, name string, invalidCode string) (int64, error) {
	raw := c.Param(name)
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		appErr := pkgerrors.ValidationFailed(invalidCode, []pkgerrors.FieldError{
			{Path: name, Code: "INVALID_VALUE"},
		})
		// 用 mergeMetadata 在 ValidationFailed 写入的 fields/count 之上追加 param/value/reason
		// 辅助字段（前端可选展开给开发者看，主 i18n 路径仍走 fields）。
		merged := mergeMetadata(appErr.Metadata, map[string]string{
			"param":  name,
			"value":  raw,
			"reason": err.Error(),
		})
		return 0, appErr.WithMetadata(merged)
	}
	return v, nil
}

// BindJSONOrError 将 gin 请求体反序列化到 req，失败返回字段级 ValidationFailed。
//
// 当底层 err 是 validator.ValidationErrors（go-playground/validator/v10 的 binding 校验失败）：
// 把每个 FieldError.Field/Tag 转 pkgerrors.FieldError{Path, Code}，code 用 tag 大写
// （required → REQUIRED、min → MIN、max → MAX、oneof → ONEOF 等）。
//
// 当底层 err 不是 validator 错（例如 JSON 语法错、类型 mismatch）：fields 为空切片，
// 仍带 reason=invalidCode 让前端按"通用 binding 错"走兜底文案；metadata.binding_error 携带
// 原始错误文本供开发者排查。
//
// invalidCode 是顶层 reason（例如 INVALID_REQUEST_BODY），与前端 i18n 表对齐。
func BindJSONOrError(c *gin.Context, req any, invalidCode string) error {
	err := c.ShouldBindJSON(req)
	if err == nil {
		return nil
	}
	fields := extractValidatorFields(err)
	appErr := pkgerrors.ValidationFailed(invalidCode, fields)
	// 在 ValidationFailed 已经写好的 fields/count 之上追加 binding_error 辅助字段，
	// 让前端可选择展开给开发者看（不影响主 i18n 路径）。
	merged := mergeMetadata(appErr.Metadata, map[string]string{"binding_error": err.Error()})
	return appErr.WithMetadata(merged)
}

// extractValidatorFields 把 go-playground/validator/v10 的 ValidationErrors 转成 FieldError 列表。
//
// 转换规则：
//   - Path 用 fe.Namespace() 去掉根类型前缀（例如 "CreateRequest.Limiters[0].LimitValue" → "limiters[0].limit_value"），
//     再 lowercase 第一段（gin binding 默认大写字段，前端期望 snake/lower）
//   - Code 用 strings.ToUpper(fe.Tag())：required → REQUIRED / min → MIN / max → MAX / oneof → ONEOF
//
// 不是 validator.ValidationErrors（json 语法错、io 错、类型 mismatch 等）返回 nil 切片，
// caller 会带空 fields + binding_error metadata 走兜底。
func extractValidatorFields(err error) []pkgerrors.FieldError {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return nil
	}
	out := make([]pkgerrors.FieldError, 0, len(ve))
	for _, fe := range ve {
		out = append(out, pkgerrors.FieldError{
			Path: validatorFieldPath(fe),
			Code: strings.ToUpper(fe.Tag()),
		})
	}
	return out
}

// validatorFieldPath 把 validator FieldError 的 Namespace 转成前端约定的 JSON 路径格式。
//
// 输入示例：
//   - "CreateRequest.Name" → "name"
//   - "CreateRequest.Limiters[0].LimitValue" → "limiters[0].limit_value"
//
// 处理：
//  1. 删掉第一段（根类型名，对前端无意义）
//  2. 把每段首字母小写化（gin binding 字段是 Go 大写，前端 JSON 是小写）
//
// 注意：完整 snake_case 转换需要驼峰拆分，这里只做"首字母小写"作为第一近似——业务里大部分
// 字段在 binding tag 中已经标了 json 名（小写），fe.Field() 拿的就是 json 名；Namespace 仅在
// 嵌套结构中用 Go 字段名拼。后续如需要严格 snake_case 可在此 helper 内升级而不影响调用方。
func validatorFieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if idx := strings.Index(ns, "."); idx >= 0 {
		ns = ns[idx+1:]
	}
	parts := strings.Split(ns, ".")
	for i, p := range parts {
		parts[i] = lowerFirstSegment(p)
	}
	return strings.Join(parts, ".")
}

// lowerFirstSegment 把段首字母小写化，保留 [index] 后缀。
func lowerFirstSegment(s string) string {
	if s == "" {
		return s
	}
	// 处理 "Limiters[0]" → 找到 '[' 位置，前缀首字母小写，后缀保留
	bracket := strings.IndexByte(s, '[')
	if bracket < 0 {
		return strings.ToLower(s[:1]) + s[1:]
	}
	prefix := s[:bracket]
	suffix := s[bracket:]
	if prefix == "" {
		return s
	}
	return strings.ToLower(prefix[:1]) + prefix[1:] + suffix
}

// mergeMetadata 把 extra 的键合并到 base，返回新 map。base 优先（不被覆盖），
// 用于在 ValidationFailed 已写入的 fields/count 之上追加辅助字段而不破坏前端解析。
func mergeMetadata(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range extra {
		out[k] = v
	}
	for k, v := range base {
		out[k] = v
	}
	return out
}

// snapshotPathIDsForOwner 在删除 channel/group/account 之前抓一份 service_quota_paths.id 快照。
//
// 为什么必须先抓：FK CASCADE 在 DELETE channel/group/account 时连带删 service_quota_paths
// 的引用行，删完之后查不到，导致后续 ResetCountersForPaths 拿到空列表 → Redis 残留 counter key
// 静默漏清。所以约定调用顺序为 "snapshot → delete → reset"。
//
// 失败仅 warn 不抛：清理 Redis 计数是 best-effort（TTL 仍兜底），不能因为这个查询失败
// 阻塞主删除流程。serviceQuotaSvc 为 nil 时直接返回 nil（service quota 模块未启用）。
func snapshotPathIDsForOwner(c *gin.Context, serviceQuotaSvc service.ServiceQuotaService, owner string, ownerID int64) []int64 {
	if serviceQuotaSvc == nil {
		return nil
	}
	pathIDs, err := serviceQuotaSvc.SnapshotPathIDsByOwner(c.Request.Context(), owner, ownerID)
	if err != nil {
		// 静默漏清比阻塞主删除流程更糟，所以只 warn；后续 ResetCountersForPaths 拿空列表自然 noop。
		slog.WarnContext(c.Request.Context(), "snapshotPathIDsForOwner failed",
			"owner", owner, "owner_id", ownerID, "err", err)
		return nil
	}
	return pathIDs
}
