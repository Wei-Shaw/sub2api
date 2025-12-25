package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

// usageCache 用于缓存usage数据
type usageCache struct {
	data      *UsageInfo
	timestamp time.Time
}

var (
	usageCacheMap = sync.Map{}
	cacheTTL      = 10 * time.Minute
)

// WindowStats 窗口期统计
type WindowStats struct {
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	Cost     float64 `json:"cost"`
}

// UsageProgress 使用量进度
type UsageProgress struct {
	Utilization      float64      `json:"utilization"`            // 使用率百分比 (0-100+，100表示100%)
	ResetsAt         *time.Time   `json:"resets_at"`              // 重置时间
	RemainingSeconds int          `json:"remaining_seconds"`      // 距重置剩余秒数
	WindowStats      *WindowStats `json:"window_stats,omitempty"` // 窗口期统计（从窗口开始到当前的使用量）
}

// UsageInfo 账号使用量信息
type UsageInfo struct {
	UpdatedAt      *time.Time     `json:"updated_at,omitempty"`       // 更新时间
	FiveHour       *UsageProgress `json:"five_hour"`                  // 5小时窗口
	SevenDay       *UsageProgress `json:"seven_day,omitempty"`        // 7天窗口
	SevenDaySonnet *UsageProgress `json:"seven_day_sonnet,omitempty"` // 7天Sonnet窗口
}

// ClaudeUsageResponse Anthropic API返回的usage结构
type ClaudeUsageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
	SevenDaySonnet struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day_sonnet"`
}

// ClaudeUsageFetcher fetches usage data from Anthropic OAuth API
type ClaudeUsageFetcher interface {
	FetchUsage(ctx context.Context, accessToken, proxyURL string) (*ClaudeUsageResponse, error)
}

// AccountUsageService 账号使用量查询服务
type AccountUsageService struct {
	accountRepo  ports.AccountRepository
	usageLogRepo ports.UsageLogRepository
	usageFetcher ClaudeUsageFetcher
}

// NewAccountUsageService 创建AccountUsageService实例
func NewAccountUsageService(accountRepo ports.AccountRepository, usageLogRepo ports.UsageLogRepository, usageFetcher ClaudeUsageFetcher) *AccountUsageService {
	return &AccountUsageService{
		accountRepo:  accountRepo,
		usageLogRepo: usageLogRepo,
		usageFetcher: usageFetcher,
	}
}

// GetUsage 获取账号使用量
// OAuth账号: 调用Anthropic API获取真实数据（需要profile scope），缓存10分钟
// Setup Token账号: 根据session_window推算5h窗口，7d数据不可用（没有profile scope）
// API Key账号: 不支持usage查询
func (s *AccountUsageService) GetUsage(ctx context.Context, accountID int64) (*UsageInfo, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account failed: %w", err)
	}

	// 只有oauth类型账号可以通过API获取usage（有profile scope）
	if account.CanGetUsage() {
		// 检查缓存
		if cached, ok := usageCacheMap.Load(accountID); ok {
			cache, ok := cached.(*usageCache)
			if !ok {
				usageCacheMap.Delete(accountID)
			} else if time.Since(cache.timestamp) < cacheTTL {
				return cache.data, nil
			}
		}

		// 从API获取数据
		usage, err := s.fetchOAuthUsage(ctx, account)
		if err != nil {
			return nil, err
		}

		// 添加5h窗口统计数据
		s.addWindowStats(ctx, account, usage)

		// 缓存结果
		usageCacheMap.Store(accountID, &usageCache{
			data:      usage,
			timestamp: time.Now(),
		})

		return usage, nil
	}

	// Setup Token账号：根据session_window推算（没有profile scope，无法调用usage API）
	if account.Type == model.AccountTypeSetupToken {
		usage := s.estimateSetupTokenUsage(account)
		// 添加窗口统计
		s.addWindowStats(ctx, account, usage)
		return usage, nil
	}

	// API Key账号不支持usage查询
	return nil, fmt.Errorf("account type %s does not support usage query", account.Type)
}

// addWindowStats 为usage数据添加窗口期统计
func (s *AccountUsageService) addWindowStats(ctx context.Context, account *model.Account, usage *UsageInfo) {
	if usage.FiveHour == nil {
		return
	}

	// 使用session_window_start作为统计起始时间
	var startTime time.Time
	if account.SessionWindowStart != nil {
		startTime = *account.SessionWindowStart
	} else {
		// 如果没有窗口信息，使用5小时前作为默认
		startTime = time.Now().Add(-5 * time.Hour)
	}

	stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, startTime)
	if err != nil {
		log.Printf("Failed to get window stats for account %d: %v", account.ID, err)
		return
	}

	usage.FiveHour.WindowStats = &WindowStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
	}
}

// GetTodayStats 获取账号今日统计
func (s *AccountUsageService) GetTodayStats(ctx context.Context, accountID int64) (*WindowStats, error) {
	stats, err := s.usageLogRepo.GetAccountTodayStats(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get today stats failed: %w", err)
	}

	return &WindowStats{
		Requests: stats.Requests,
		Tokens:   stats.Tokens,
		Cost:     stats.Cost,
	}, nil
}

func (s *AccountUsageService) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	stats, err := s.usageLogRepo.GetAccountUsageStats(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get account usage stats failed: %w", err)
	}
	return stats, nil
}

// fetchOAuthUsage 从Anthropic API获取OAuth账号的使用量
func (s *AccountUsageService) fetchOAuthUsage(ctx context.Context, account *model.Account) (*UsageInfo, error) {
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("no access token available")
	}

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	usageResp, err := s.usageFetcher.FetchUsage(ctx, accessToken, proxyURL)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return s.buildUsageInfo(usageResp, &now), nil
}

// parseTime 尝试多种格式解析时间
func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}

// buildUsageInfo 构建UsageInfo
func (s *AccountUsageService) buildUsageInfo(resp *ClaudeUsageResponse, updatedAt *time.Time) *UsageInfo {
	info := &UsageInfo{
		UpdatedAt: updatedAt,
	}

	// 5小时窗口
	if resp.FiveHour.ResetsAt != "" {
		if fiveHourReset, err := parseTime(resp.FiveHour.ResetsAt); err == nil {
			info.FiveHour = &UsageProgress{
				Utilization:      resp.FiveHour.Utilization,
				ResetsAt:         &fiveHourReset,
				RemainingSeconds: int(time.Until(fiveHourReset).Seconds()),
			}
		} else {
			log.Printf("Failed to parse FiveHour.ResetsAt: %s, error: %v", resp.FiveHour.ResetsAt, err)
			// 即使解析失败也返回utilization
			info.FiveHour = &UsageProgress{
				Utilization: resp.FiveHour.Utilization,
			}
		}
	}

	// 7天窗口
	if resp.SevenDay.ResetsAt != "" {
		if sevenDayReset, err := parseTime(resp.SevenDay.ResetsAt); err == nil {
			info.SevenDay = &UsageProgress{
				Utilization:      resp.SevenDay.Utilization,
				ResetsAt:         &sevenDayReset,
				RemainingSeconds: int(time.Until(sevenDayReset).Seconds()),
			}
		} else {
			log.Printf("Failed to parse SevenDay.ResetsAt: %s, error: %v", resp.SevenDay.ResetsAt, err)
			info.SevenDay = &UsageProgress{
				Utilization: resp.SevenDay.Utilization,
			}
		}
	}

	// 7天Sonnet窗口
	if resp.SevenDaySonnet.ResetsAt != "" {
		if sonnetReset, err := parseTime(resp.SevenDaySonnet.ResetsAt); err == nil {
			info.SevenDaySonnet = &UsageProgress{
				Utilization:      resp.SevenDaySonnet.Utilization,
				ResetsAt:         &sonnetReset,
				RemainingSeconds: int(time.Until(sonnetReset).Seconds()),
			}
		} else {
			log.Printf("Failed to parse SevenDaySonnet.ResetsAt: %s, error: %v", resp.SevenDaySonnet.ResetsAt, err)
			info.SevenDaySonnet = &UsageProgress{
				Utilization: resp.SevenDaySonnet.Utilization,
			}
		}
	}

	return info
}

// estimateSetupTokenUsage 根据session_window推算Setup Token账号的使用量
func (s *AccountUsageService) estimateSetupTokenUsage(account *model.Account) *UsageInfo {
	info := &UsageInfo{}

	// 如果有session_window信息
	if account.SessionWindowEnd != nil {
		remaining := int(time.Until(*account.SessionWindowEnd).Seconds())
		if remaining < 0 {
			remaining = 0
		}

		// 根据状态估算使用率 (百分比形式，100 = 100%)
		var utilization float64
		switch account.SessionWindowStatus {
		case "rejected":
			utilization = 100.0
		case "allowed_warning":
			utilization = 80.0
		default:
			utilization = 0.0
		}

		info.FiveHour = &UsageProgress{
			Utilization:      utilization,
			ResetsAt:         account.SessionWindowEnd,
			RemainingSeconds: remaining,
		}
	} else {
		// 没有窗口信息，返回空数据
		info.FiveHour = &UsageProgress{
			Utilization:      0,
			RemainingSeconds: 0,
		}
	}

	// Setup Token无法获取7d数据
	return info
}
