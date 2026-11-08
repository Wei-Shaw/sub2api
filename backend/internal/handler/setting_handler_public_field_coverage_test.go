//go:build unit

package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
)

// TestGetPublicSettings_AssignsEveryDTOField 防止「dto 有字段、handler 忘记搬运」的静默丢字段。
//
// 为什么需要这条测试：handler 用 struct literal 把 service.PublicSettings 逐字段搬进
// dto.PublicSettings。Go 的 composite literal 允许漏字段（漏了就是零值），**编译器不会报错**。
// intelligence_check_enabled 就这样真实漏过一次：
// 数据库里是 true、service 也算出了 true，但 handler 没搬运，于是 /api/v1/settings/public
// 永远返回 false —— 前端 feature flag 恒判假，用户端「智力检测」入口永久隐藏，
// 而且和系统设置里的真实开关状态彻底脱节，排查时完全看不出问题在映射层。
//
// 已有的 public_settings_injection_schema_test.go 只比对**字段集合**（声明在不在），
// 拦不住这种「声明在、值没赋」的情况，所以补这一条覆盖赋值完整性。
func TestGetPublicSettings_AssignsEveryDTOField(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "setting_handler.go", nil, 0)
	if err != nil {
		t.Fatalf("解析 setting_handler.go 失败: %v", err)
	}

	assigned := map[string]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "dto" || sel.Sel.Name != "PublicSettings" {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok {
				assigned[key.Name] = struct{}{}
			}
		}
		return true
	})

	if len(assigned) == 0 {
		t.Fatal("未在 setting_handler.go 中找到 dto.PublicSettings 的 composite literal，测试需要同步更新")
	}

	typ := reflect.TypeOf(dto.PublicSettings{})
	var missing []string
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, ok := assigned[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("GetPublicSettings 未给这些 dto.PublicSettings 字段赋值: %s\n"+
			"漏字段不会编译报错、只会静默取零值，前端会读到错误的开关状态。"+
			"请补上搬运，或在此测试中显式记录排除原因。", strings.Join(missing, ", "))
	}
}
