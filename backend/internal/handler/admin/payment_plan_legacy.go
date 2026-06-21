package admin

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func legacySubscriptionPlans(plans []*dbent.SubscriptionPlan, groupInfo map[int64]service.PlanGroupInfo, admin bool) []gin.H {
	out := make([]gin.H, 0, len(plans))
	for _, p := range plans {
		gi := groupInfo[p.GroupID]
		item := gin.H{
			"id":                    int64(p.ID),
			"groupId":               p.GroupID,
			"groupName":             gi.Name,
			"name":                  p.Name,
			"description":           p.Description,
			"price":                 p.Price,
			"originalPrice":         p.OriginalPrice,
			"validityDays":          p.ValidityDays,
			"validityUnit":          p.ValidityUnit,
			"features":              parseFeatures(p.Features),
			"productName":           p.ProductName,
			"platform":              gi.Platform,
			"rateMultiplier":        gi.RateMultiplier,
			"allowMessagesDispatch": false,
			"defaultMappedModel":    "",
			"limits": gin.H{
				"daily_limit_usd":   gi.DailyLimitUSD,
				"weekly_limit_usd":  gi.WeeklyLimitUSD,
				"monthly_limit_usd": gi.MonthlyLimitUSD,
			},
		}
		if admin {
			item["validDays"] = p.ValidityDays
			item["sortOrder"] = p.SortOrder
			item["enabled"] = p.ForSale
			item["groupExists"] = gi.Name != ""
			item["groupPlatform"] = gi.Platform
			item["groupRateMultiplier"] = gi.RateMultiplier
			item["groupDailyLimit"] = gi.DailyLimitUSD
			item["groupWeeklyLimit"] = gi.WeeklyLimitUSD
			item["groupMonthlyLimit"] = gi.MonthlyLimitUSD
			item["groupModelScopes"] = gi.ModelScopes
			item["createdAt"] = p.CreatedAt
			item["updatedAt"] = p.UpdatedAt
		}
		out = append(out, item)
	}
	return out
}

func parseFeatures(raw string) []string {
	if raw == "" {
		return []string{}
	}
	out := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

type legacyPlanUpsertRequest struct {
	GroupID       *int64    `json:"group_id"`
	Name          *string   `json:"name"`
	Description   *string   `json:"description"`
	Price         *float64  `json:"price"`
	OriginalPrice *float64  `json:"original_price"`
	ValidityDays  *float64  `json:"validity_days"`
	ValidityUnit  *string   `json:"validity_unit"`
	Features      *[]string `json:"features"`
	ProductName   *string   `json:"product_name"`
	ForSale       *bool     `json:"for_sale"`
	SortOrder     *int      `json:"sort_order"`
}

func flattenLegacyFeatures(features []string) string {
	if len(features) == 0 {
		return ""
	}
	clean := make([]string, 0, len(features))
	for _, item := range features {
		if s := strings.TrimSpace(item); s != "" {
			clean = append(clean, s)
		}
	}
	return strings.Join(clean, "\n")
}
