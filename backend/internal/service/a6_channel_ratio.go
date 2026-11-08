package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A6ChannelRatioKey 标识「某个 A6 渠道上的某个模型」。
//
// 为什么要带模型：A6 的渠道倍率是**按渠道 + 模型**给的（同一渠道的不同模型
// realtime_ratio_usd 并不相同，实测渠道 1926 的三个模型分别是
// 0.006948 / 0.0066996 / 0.008346）。
type A6ChannelRatioKey struct {
	ChannelID int
	Model     string
}

// a6ChannelRatioCacheTTL 渠道倍率的缓存时长。
//
// 倍率不会秒级变化，而使用记录一页可能有几十个不同渠道；不缓存的话每翻一页
// 都要对 A6 发几十次请求。10 分钟足够新鲜，也足够省。
const a6ChannelRatioCacheTTL = 10 * time.Minute

// a6ChannelRatioTimeout 单次查询超时（公开接口，很快）。
const a6ChannelRatioTimeout = 10 * time.Second

type a6ChannelRatioCacheEntry struct {
	ratios    map[string]float64 // model → realtime_ratio_usd
	expiresAt time.Time
}

// A6ChannelRatioService 查询 A6 渠道的实时倍率（realtime_ratio_usd）。
//
// 数据来源是 A6 的**公开**市场接口
//
//	GET {base}/api/marketplace/public/channels/search?channel_id=<id>
//
// 该接口不需要鉴权，所以这里**刻意不带 Authorization**：带着一个过期的面板令牌
// 去请求公开接口，反而可能被上游以 401 拒掉，把一个纯展示用的功能变成新的故障点。
type A6ChannelRatioService struct {
	settings *ReconciliationA6SettingsService
	client   *http.Client

	mu    sync.Mutex
	cache map[int]a6ChannelRatioCacheEntry
}

// NewA6ChannelRatioService 构造渠道倍率查询服务。
func NewA6ChannelRatioService(settings *ReconciliationA6SettingsService) *A6ChannelRatioService {
	return &A6ChannelRatioService{
		settings: settings,
		client:   &http.Client{Timeout: a6ChannelRatioTimeout},
		cache:    make(map[int]a6ChannelRatioCacheEntry),
	}
}

// Resolve 批量解析渠道倍率，返回 map["<channelID>|<model>"]倍率。
//
// 取不到的键不会出现在结果里（调用方按「没有就显示 —」处理）：
// 上游超时、渠道已下架、模型对不上都属于正常情况，不该让整页使用记录失败。
func (s *A6ChannelRatioService) Resolve(ctx context.Context, keys []A6ChannelRatioKey) map[string]float64 {
	out := make(map[string]float64, len(keys))
	if s == nil || s.settings == nil || len(keys) == 0 {
		return out
	}
	base := strings.TrimRight(strings.TrimSpace(s.settings.Effective(ctx).BaseURL), "/")
	if base == "" {
		return out
	}

	byChannel := make(map[int][]string, len(keys))
	for _, k := range keys {
		if k.ChannelID <= 0 || strings.TrimSpace(k.Model) == "" {
			continue
		}
		byChannel[k.ChannelID] = append(byChannel[k.ChannelID], strings.TrimSpace(k.Model))
	}

	for channelID, models := range byChannel {
		ratios, err := s.channelRatios(ctx, base, channelID)
		if err != nil {
			continue
		}
		for _, model := range models {
			if v, ok := ratios[model]; ok {
				out[a6ChannelRatioKeyString(channelID, model)] = v
			}
		}
	}
	return out
}

// ChannelRatio 是 Resolve 的单项便捷入口。
func (s *A6ChannelRatioService) ChannelRatio(ctx context.Context, channelID int, model string) (float64, bool) {
	m := s.Resolve(ctx, []A6ChannelRatioKey{{ChannelID: channelID, Model: model}})
	v, ok := m[a6ChannelRatioKeyString(channelID, model)]
	return v, ok
}

func a6ChannelRatioKeyString(channelID int, model string) string {
	return strconv.Itoa(channelID) + "|" + model
}

// channelRatios 取单个渠道的「模型 → 倍率」表，带缓存。
func (s *A6ChannelRatioService) channelRatios(ctx context.Context, base string, channelID int) (map[string]float64, error) {
	now := time.Now()
	s.mu.Lock()
	if e, ok := s.cache[channelID]; ok && now.Before(e.expiresAt) {
		s.mu.Unlock()
		return e.ratios, nil
	}
	s.mu.Unlock()

	ratios, err := s.fetchChannelRatios(ctx, base, channelID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[channelID] = a6ChannelRatioCacheEntry{ratios: ratios, expiresAt: now.Add(a6ChannelRatioCacheTTL)}
	s.mu.Unlock()
	return ratios, nil
}

func (s *A6ChannelRatioService) fetchChannelRatios(ctx context.Context, base string, channelID int) (map[string]float64, error) {
	endpoint := fmt.Sprintf("%s/api/marketplace/public/channels/search?channel_id=%s&page=1&page_size=100",
		base, url.QueryEscape(strconv.Itoa(channelID)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sub2api-admin-usage")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16)); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a6 channel ratio: unexpected status %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			Items []struct {
				ModelName        string   `json:"model_name"`
				RealtimeRatioUSD *float64 `json:"realtime_ratio_usd"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}

	out := make(map[string]float64, len(envelope.Data.Items))
	for _, item := range envelope.Data.Items {
		if item.RealtimeRatioUSD == nil {
			continue
		}
		model := strings.TrimSpace(item.ModelName)
		if model == "" {
			continue
		}
		out[model] = *item.RealtimeRatioUSD
	}
	return out, nil
}
