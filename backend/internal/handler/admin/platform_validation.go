package admin

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// 平台相关的请求绑定校验标签，由平台清单（domain/platforms.go）驱动，替代在
// binding 标签里手写 oneof 白名单（新增平台时易漏加，导致分组无法创建）：
//   - group_platform：已登记的具体平台或 composite；
//   - concrete_platform：已登记的具体平台；
//   - composite_route_target：可作组合路由目标的具体平台（排除 Cursor，
//     其 picker 模型名与 grok 等平台重叠）。
func init() {
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		panic("platform validation requires a go-playground validator engine")
	}
	if err := engine.RegisterValidation("group_platform", func(fl validator.FieldLevel) bool {
		return domain.IsGroupPlatform(fl.Field().String())
	}); err != nil {
		panic(fmt.Errorf("register group_platform validation: %w", err))
	}
	if err := engine.RegisterValidation("concrete_platform", func(fl validator.FieldLevel) bool {
		return domain.IsConcretePlatform(fl.Field().String())
	}); err != nil {
		panic(fmt.Errorf("register concrete_platform validation: %w", err))
	}
	if err := engine.RegisterValidation("composite_route_target", func(fl validator.FieldLevel) bool {
		return domain.IsCompositeRouteTarget(fl.Field().String())
	}); err != nil {
		panic(fmt.Errorf("register composite_route_target validation: %w", err))
	}
}
