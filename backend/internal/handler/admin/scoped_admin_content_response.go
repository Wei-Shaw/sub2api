package admin

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// 受限密钥仅接收记录自身字段；不嵌入完整用户资料或实时财务数据。
type scopedPromoUsageResponse struct {
	ID          int64     `json:"id"`
	PromoCodeID int64     `json:"promo_code_id"`
	UserID      int64     `json:"user_id"`
	BonusAmount float64   `json:"bonus_amount"`
	UsedAt      time.Time `json:"used_at"`
}

func adminPromoUsagesResponse(c *gin.Context, items []dto.PromoCodeUsage) any {
	if adminaccess.FromContext(c.Request.Context()) == nil {
		return items
	}
	out := make([]scopedPromoUsageResponse, 0, len(items))
	for _, item := range items {
		out = append(out, scopedPromoUsageResponse{ID: item.ID, PromoCodeID: item.PromoCodeID, UserID: item.UserID, BonusAmount: item.BonusAmount, UsedAt: item.UsedAt})
	}
	return out
}

type scopedAnnouncementReadResponse struct {
	UserID   int64      `json:"user_id"`
	Eligible bool       `json:"eligible"`
	ReadAt   *time.Time `json:"read_at,omitempty"`
}

func adminAnnouncementReadResponse(c *gin.Context, items []service.AnnouncementUserReadStatus) any {
	if adminaccess.FromContext(c.Request.Context()) == nil {
		return items
	}
	out := make([]scopedAnnouncementReadResponse, 0, len(items))
	for _, item := range items {
		out = append(out, scopedAnnouncementReadResponse{UserID: item.UserID, Eligible: item.Eligible, ReadAt: item.ReadAt})
	}
	return out
}
