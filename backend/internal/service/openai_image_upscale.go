package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const maxConcurrentOpenAIImageUpscales = 4

// rewriteOpenAIImageBase64InBody 把响应体里发生改动的 base64 图片字符串替换为放大后的版本。
// 采用字面量替换：base64 标准字母表（A-Za-z0-9+/=）在 JSON 字符串中无需转义、且单张 b64 很长且唯一，
// 因此按 old→new 逐张替换既与响应形状无关（data[]/output[]、b64_json/result 通吃），也无需重新 marshal。
func rewriteOpenAIImageBase64InBody(body []byte, oldB64s, newB64s []string) []byte {
	out := body
	n := len(oldB64s)
	if len(newB64s) < n {
		n = len(newB64s)
	}
	for i := 0; i < n; i++ {
		o, nv := oldB64s[i], newB64s[i]
		if o == "" || nv == "" || o == nv {
			continue
		}
		out = bytes.Replace(out, []byte(o), []byte(nv), 1)
	}
	return out
}

// requestedUpscaleTargetSize 返回用于 upscale 目标档位判定的「显式请求尺寸」。
// 仅当用户显式指定了 size 时返回该值；否则返回空（不触发放大，符合「请求了 2K 以上」语义，
// 避免 auto 请求被默认归一到 2K 而误放大）。
func requestedUpscaleTargetSize(parsed *OpenAIImagesRequest) string {
	if parsed == nil || !parsed.ExplicitSize {
		return ""
	}
	return parsed.Size
}

// upscaleFactorForTarget 返回从真实尺寸放大到目标档位标准长边所需的倍数；0 表示无需/无法放大。
// 标准长边：1K=1024、2K=2048、4K=3840。factor 最大允许 10。
func upscaleFactorForTarget(realSize, targetTier string) int {
	targetLongEdge := upscaleTargetLongEdge(targetTier)
	if targetLongEdge <= 0 {
		return 0
	}
	width, height, ok := parseImageBillingDimensions(realSize)
	if !ok {
		return 0
	}
	realLongEdge := width
	if height > realLongEdge {
		realLongEdge = height
	}
	if realLongEdge >= targetLongEdge {
		return 0
	}
	factor := (targetLongEdge + realLongEdge - 1) / realLongEdge
	if factor < 2 || factor > 10 {
		return 0
	}
	return factor
}

func upscaleTargetLongEdge(tier string) int {
	switch tier {
	case ImageBillingSize1K:
		return 1024
	case ImageBillingSize2K:
		return 2048
	case ImageBillingSize4K:
		return 3840
	default:
		return 0
	}
}

// maybeUpscaleOpenAIImages 在「分组开关开 + 目标档位≥2K + 系统配置就绪」时，对低于目标档位的
// b64 图调用 fal SeedVR upscale 放大到目标档位。
// 返回（可能被替换的）b64、sizes、fal fallback URLs 切片及是否有改动。
//
// sizes 与 b64s 同序、同长度；放大成功的 slot 会把 size 更新为放大后真实尺寸，使后续计费按目标档位结算。
// fallback URLs 与 b64s 同序；仅放大成功的 slot 填入 fal 返回的图片 URL，供 status.urls 在 COS 不可用时兜底。
//
// 安全保证：分组未开 / 目标档位不达标 / 未配置 / 解码失败 / 放大失败 / 超时 —— 一律保留原图与原 size，
// 不报错（由调用方按原图返回与计费，自洽满足兜底语义）。
func (s *OpenAIGatewayService) maybeUpscaleOpenAIImages(
	ctx context.Context, group *Group, requestedSize string, b64s, sizes []string,
) (outB64 []string, outSizes []string, fallbackURLs []string, changed bool) {
	outB64 = b64s
	outSizes = sizes
	if s == nil || s.settingService == nil || group == nil || len(b64s) == 0 {
		return outB64, outSizes, nil, false
	}
	if !group.ImageUpscaleOnRspEnabled() {
		return outB64, outSizes, nil, false
	}
	targetTier, ok := ClassifyImageBillingTier(requestedSize)

	// OpenAI的反代阉割了2K以及以上的生图, 故仅当显式请求了 ≥2K 才触发放大；否则按原图档位计费。
	if !ok || imageTierRank(targetTier) < imageTierRank(ImageBillingSize2K) {
		return outB64, outSizes, nil, false
	}
	cfg := s.settingService.GetFalUpscaleSettings(ctx)
	if !cfg.Configured() {
		return outB64, outSizes, nil, false
	}
	client, err := fal.NewClient(fal.Config{APIKey: cfg.Token})
	if err != nil {
		logger.L().Warn("openai.images.upscale.client_init_failed", zap.Error(err))
		return outB64, outSizes, nil, false
	}

	resultB64 := make([]string, len(b64s))
	copy(resultB64, b64s)
	resultSizes := make([]string, len(b64s))
	copy(resultSizes, sizes)
	resultURLs := make([]string, len(b64s))

	var wg sync.WaitGroup
	var changedMu sync.Mutex
	sem := make(chan struct{}, maxConcurrentOpenAIImageUpscales)

	for i, b64 := range b64s {
		if strings.TrimSpace(b64) == "" {
			continue
		}
		sizeStr, derr := decodeImageSizeFromBase64(b64)
		if derr != nil {
			continue
		}
		realTier, rok := ClassifyImageBillingTier(sizeStr)
		if !rok {
			continue
		}
		factor := upscaleFactorForTarget(sizeStr, targetTier)
		if factor == 0 {
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(i int, b64, sizeStr, realTier string, factor int) {
			defer wg.Done()
			defer func() { <-sem }()

			logger.L().Debug("openai.images.upscale.begin",
				zap.Int("index", i),
				zap.String("real_sizeStr", sizeStr),
				zap.String("real_tier", realTier),
				zap.String("target_tier", targetTier),
				zap.Int("factor", factor),
			)

			newB64, falURL, uerr := s.runSeedVRUpscale(ctx, client, cfg, b64, factor)
			if uerr != nil {
				logger.L().Error("openai.images.upscale.failed",
					zap.Int("index", i),
					zap.String("real_tier", realTier),
					zap.String("target_tier", targetTier),
					zap.Int("factor", factor),
					zap.Error(uerr),
				)
				return // 兜底原图
			}
			resultB64[i] = newB64
			resultURLs[i] = falURL
			// 用放大后真实尺寸更新 size，使计费按目标档位结算（成功按目标、失败保留原图档位）。
			if newSize, e := decodeImageSizeFromBase64(newB64); e == nil {
				resultSizes[i] = newSize
			}
			changedMu.Lock()
			changed = true
			changedMu.Unlock()
			logger.L().Info("openai.images.upscale.ok",
				zap.Int("index", i),
				zap.String("real_tier", realTier),
				zap.String("target_tier", targetTier),
				zap.Int("factor", factor),
			)
		}(i, b64, sizeStr, realTier, factor)
	}
	wg.Wait()
	if !changed {
		resultURLs = nil
	}
	return resultB64, resultSizes, resultURLs, changed
}

// runSeedVRUpscale 对单张 b64 图调用 fal SeedVR upscale（queue 轮询 + 结果下载），返回放大后的 b64 与 fal 图片 URL。
func (s *OpenAIGatewayService) runSeedVRUpscale(
	ctx context.Context, client *fal.Client, cfg *FalUpscaleSettings, b64 string, factor int,
) (string, string, error) {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(defaultFalUpscaleTimeoutSeconds) * time.Second
	}
	upCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dataURI := "data:image/png;base64," + b64
	sub, err := client.SubmitUpscale(upCtx, cfg.Endpoint, &fal.UpscaleRequest{
		ImageURL:      dataURI,
		UpscaleMode:   "factor",
		UpscaleFactor: factor,
		OutputFormat:  "png",
	})
	if err != nil {
		return "", "", fmt.Errorf("submit: %w", err)
	}

	statusURL := strings.TrimSpace(sub.StatusURL)
	if statusURL == "" {
		statusURL = client.BuildStatusURL(cfg.Endpoint, sub.RequestID)
	}
	responseURL := strings.TrimSpace(sub.ResponseURL)
	if responseURL == "" {
		responseURL = client.BuildResponseURL(cfg.Endpoint, sub.RequestID)
	}

	// 轮询直到完成或超时。
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		st, serr := client.Status(upCtx, statusURL)
		if serr != nil {
			return "", "", fmt.Errorf("status: %w", serr)
		}
		if st.Status == fal.StatusCompleted {
			if u := strings.TrimSpace(st.ResponseURL); u != "" {
				responseURL = u
			}
			break
		}
		select {
		case <-upCtx.Done():
			return "", "", fmt.Errorf("upscale timeout: %w", upCtx.Err())
		case <-ticker.C:
		}
	}

	res, rerr := client.UpscaleResult(upCtx, responseURL)
	if rerr != nil {
		return "", "", fmt.Errorf("result: %w", rerr)
	}
	imgURL := strings.TrimSpace(res.Image.URL)
	if imgURL == "" {
		return "", "", fmt.Errorf("upscale result has empty image url")
	}

	slog.Debug("openai.images.upscale.download", "url", imgURL)
	bytesOut, derr := downloadImageBytes(upCtx, s.upscaleHTTPClient(), imgURL)
	slog.Debug("openai.images.upscale.download done", "file bytes", len(bytesOut), "error", derr)

	if derr != nil {
		return "", "", fmt.Errorf("download: %w", derr)
	}
	return base64.StdEncoding.EncodeToString(bytesOut), imgURL, nil
}

func mergeOpenAIImageOutputURLs(primary, fallback []string) []string {
	out := make([]string, 0, len(primary)+len(fallback))
	seen := make(map[string]struct{}, len(primary)+len(fallback))
	appendURL := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	for _, u := range primary {
		appendURL(u)
	}
	for _, u := range fallback {
		appendURL(u)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// upscaleHTTPClient 返回用于下载 fal 结果图的 HTTP 客户端（复用网关的 http client，缺省 default）。
func (s *OpenAIGatewayService) upscaleHTTPClient() *http.Client {
	return http.DefaultClient
}

// downloadImageBytes 下载图片字节（受 ctx 超时约束，限制最大体积）。
func downloadImageBytes(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download status %d", resp.StatusCode)
	}
	const maxImageBytes = 64 << 20 // 64MB 上限
	return io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
}

// shouldBufferOpenAIImagesForUpscale 预判本请求是否需要为 upscale 缓冲流式响应。
// 仅当分组开关开、显式请求目标档位 ≥ 2K、且系统配置就绪时为 true。
func (s *OpenAIGatewayService) shouldBufferOpenAIImagesForUpscale(ctx context.Context, group *Group, requestedSize string) bool {
	if s == nil || s.settingService == nil || group == nil {
		return false
	}
	if !group.ImageUpscaleOnRspEnabled() {
		return false
	}
	tier, ok := ClassifyImageBillingTier(requestedSize)
	if !ok || imageTierRank(tier) < imageTierRank(ImageBillingSize2K) {
		return false
	}
	return s.settingService.GetFalUpscaleSettings(ctx).Configured()
}
