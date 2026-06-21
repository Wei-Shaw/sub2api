package handler

import (
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
