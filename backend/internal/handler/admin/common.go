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
	"strconv"

	"github.com/gin-gonic/gin"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ParseInt64Param 解析 gin path 参数为 int64，解析失败返回结构化 BadRequest。
//
// invalidCode 由调用方传入对应的错误码常量（例如 errReasonQuotaInvalidUserID），
// 与前端 i18n key 对齐；metadata 中附 param/value/reason 便于排查前端传错或非数字。
// 值范围校验（>0、上限等）留给 service 层；本函数只承担类型转换。
func ParseInt64Param(c *gin.Context, name string, invalidCode string) (int64, error) {
	raw := c.Param(name)
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, pkgerrors.BadRequest(invalidCode, "invalid "+name+" param").
			WithMetadata(map[string]string{
				"param":  name,
				"value":  raw,
				"reason": err.Error(),
			})
	}
	return v, nil
}

// BindJSONOrError 将 gin 请求体反序列化到 req，失败返回结构化 BadRequest。
//
// invalidCode 与 ParseInt64Param 同源（如 errReasonQuotaInvalidRequest），
// metadata.reason 保留 gin binding 的具体字段级报错，方便前后端联调。
func BindJSONOrError(c *gin.Context, req interface{}, invalidCode string) error {
	if err := c.ShouldBindJSON(req); err != nil {
		return pkgerrors.BadRequest(invalidCode, "invalid request body").
			WithMetadata(map[string]string{"reason": err.Error()})
	}
	return nil
}
