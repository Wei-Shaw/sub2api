package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// 平台相关的请求绑定校验标签，由平台清单（domain/platforms.go）驱动，替代在
// binding 标签里手写 oneof 白名单（新增平台时易漏加，导致分组无法创建）：
//   - group_platform：已登记的具体平台或 composite；
//   - concrete_platform：已登记的具体平台。
func init() {
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	_ = engine.RegisterValidation("group_platform", func(fl validator.FieldLevel) bool {
		return domain.IsGroupPlatform(fl.Field().String())
	})
	_ = engine.RegisterValidation("concrete_platform", func(fl validator.FieldLevel) bool {
		return domain.IsConcretePlatform(fl.Field().String())
	})
}
