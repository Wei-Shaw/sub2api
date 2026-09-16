package admin

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestRefreshSingleAccount_RejectsShadow 验证外审第6轮:手动刷新对 spark 影子在调用上游前早拒
// (影子凭据由母账号管理、自身恒空,刷新无意义)。该守卫同时覆盖单账号与批量刷新两入口。
func TestRefreshSingleAccount_RejectsShadow(t *testing.T) {
	h := &AccountHandler{} // 影子在使用任何依赖前即返回,无需注入
	parentID := int64(5)
	shadow := &service.Account{
		ID:              9,
		Platform:        service.PlatformOpenAI,
		Type:            service.AccountTypeOAuth, // IsOAuth()=true,确保不是先撞 NOT_OAUTH
		ParentAccountID: &parentID,
		QuotaDimension:  service.QuotaDimensionSpark,
	}

	_, _, err := h.refreshSingleAccount(context.Background(), shadow)
	require.Error(t, err, "影子刷新应被早拒")
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
}
