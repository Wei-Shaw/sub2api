// Package service - openai_image_payload_size.go
//
// 实现 spec media-prepay-billing「回包图片分辨率自检（base64）」需求（D8/D9）：
// 当 platform=openai 分组开启 image_decode_size_on_rsp 时，对回包中 size 缺失或 size="auto"
// 的 slot，按其 b64_json 内容做最小代价的头部解码（image.DecodeConfig 仅读取尺寸元数据，
// 不解码像素），用真实 {w}x{h} 回填 size，再交给 6 档归档计费。
//
// 仅 base64 模式：URL 模式（b64 为空）跳过；解码失败/异常一律走默认 2K 档兜底，不抛错。
package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"

	// 注册标准库 PNG/JPEG decoder。GIF 需要时再加。
	_ "image/jpeg"
	_ "image/png"
	"strings"

	// 注册 WebP decoder（标准库不带）。
	_ "golang.org/x/image/webp"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// imageDecodeMaxBase64Bytes 限制单个 b64 payload 最大解码长度（约 50 MB），
// 防御异常 payload 引发 OOM。OpenAI 实际产物远小于此（4K JPEG 约 1-3 MB）。
const imageDecodeMaxBase64Bytes = 50 * 1024 * 1024

// imageDecodeMaxRawBytes 解码后原始字节上限。base64 解码膨胀比 ~0.75，按上限同步换算保护。
const imageDecodeMaxRawBytes = imageDecodeMaxBase64Bytes * 3 / 4

// DecodeOpenAIImageOutputSizes 在异步记账阶段对回包做 size 自检。
// 仅当 group != nil 且 group 开启 image_decode_size_on_rsp 且 platform=openai 时才生效。
// 遍历 result.ImageOutputSizes，对 size ∈ {"", "auto"} 且对应 b64 非空的 slot 解码尺寸，
// 成功回填 result.ImageOutputSizes[i] = "{w}x{h}"，失败留空走默认档兜底。
//
// 解码失败按 warn 级别记 openai.images.size_decode_failed，成功不打日志（避免高吞吐刷屏）。
//
// 与 ApplyOpenAIImageBillingResolution 的契约：本函数只负责修订 ImageOutputSizes，
// 不直接修改 ImageSize/ImageSizeSource——后续 ApplyOpenAIImageBillingResolution 会基于
// 修订后的 ImageOutputSizes 重新归档并设置 ImageSizeSource = "output_decoded"（当且仅当
// 至少一个 slot 是被本函数解码出来的）。
func DecodeOpenAIImageOutputSizes(result *OpenAIForwardResult, group *Group) {
	// === DEBUG: 入口快照（定位「为什么不生效」）===
	if result == nil {
		logger.L().Info("[debug] openai.images.decode_size_on_rsp.skip",
			zap.String("component", "service.openai_gateway"),
			zap.String("reason", "result_nil"),
		)
		return
	}
	{
		var (
			groupID       int64
			groupPlatform string
			toggleEnabled bool
			groupNil      = group == nil
		)
		if group != nil {
			groupID = int64(group.ID)
			groupPlatform = group.Platform
			toggleEnabled = group.ImageDecodeSizeOnRsp
		}
		// b64 摘要：每个 slot 的字节数（不打 payload 内容，避免日志过大）
		b64Lens := make([]int, len(result.ImageOutputBase64))
		for i, p := range result.ImageOutputBase64 {
			b64Lens[i] = len(p)
		}
		logger.L().Info("[debug] openai.images.decode_size_on_rsp.enter",
			zap.String("component", "service.openai_gateway"),
			zap.Bool("group_nil", groupNil),
			zap.Int64("group_id", groupID),
			zap.String("group_platform", groupPlatform),
			zap.Bool("decode_size_on_rsp", toggleEnabled),
			zap.Int("image_count", result.ImageCount),
			zap.Strings("image_output_sizes", result.ImageOutputSizes),
			zap.Int("image_output_base64_count", len(result.ImageOutputBase64)),
			zap.Ints("image_output_base64_lens", b64Lens),
		)
	}

	if !group.ImageDecodeSizeOnRspEnabled() {
		logger.L().Info("[debug] openai.images.decode_size_on_rsp.skip",
			zap.String("component", "service.openai_gateway"),
			zap.String("reason", "toggle_disabled_or_platform_mismatch_or_group_nil"),
		)
		return
	}
	if len(result.ImageOutputSizes) == 0 || len(result.ImageOutputBase64) == 0 {
		logger.L().Info("[debug] openai.images.decode_size_on_rsp.skip",
			zap.String("component", "service.openai_gateway"),
			zap.String("reason", "empty_sizes_or_base64"),
			zap.Int("sizes_len", len(result.ImageOutputSizes)),
			zap.Int("base64_len", len(result.ImageOutputBase64)),
		)
		return
	}

	// recover 兜底：解码异常不影响调用方流程。
	defer func() {
		if r := recover(); r != nil {
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[warn] [openai.images.size_decode_failed] panic during decode: %v group_id=%d",
				r, group.ID,
			)
		}
	}()

	groupID := group.ID
	decoded := false
	for i, size := range result.ImageOutputSizes {
		if !needDecodeImageSize(size) {
			logger.L().Info("[debug] openai.images.decode_size_on_rsp.slot_skip",
				zap.String("component", "service.openai_gateway"),
				zap.Int("slot_index", i),
				zap.String("size", size),
				zap.String("reason", "size_already_resolved"),
			)
			continue
		}
		if i >= len(result.ImageOutputBase64) {
			logger.L().Info("[debug] openai.images.decode_size_on_rsp.slot_skip",
				zap.String("component", "service.openai_gateway"),
				zap.Int("slot_index", i),
				zap.String("reason", "no_matching_base64_slot"),
				zap.Int("base64_len", len(result.ImageOutputBase64)),
			)
			continue
		}
		b64Payload := strings.TrimSpace(result.ImageOutputBase64[i])
		if b64Payload == "" {
			// URL 模式或上游未携带 b64：保持空，走默认档兜底（spec scenario）。
			logger.L().Info("[debug] openai.images.decode_size_on_rsp.slot_skip",
				zap.String("component", "service.openai_gateway"),
				zap.Int("slot_index", i),
				zap.String("reason", "base64_empty_url_mode"),
			)
			continue
		}
		decodedSize, err := decodeImageSizeFromBase64(b64Payload)
		if err != nil {
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[warn] [openai.images.size_decode_failed] slot_index=%d bytes=%d error=%v group_id=%d",
				i, len(b64Payload), err, groupID,
			)
			continue
		}
		logger.L().Info("[debug] openai.images.decode_size_on_rsp.slot_decoded",
			zap.String("component", "service.openai_gateway"),
			zap.Int("slot_index", i),
			zap.String("size_before", size),
			zap.String("size_after", decodedSize),
			zap.Int("bytes", len(b64Payload)),
		)
		result.ImageOutputSizes[i] = decodedSize
		decoded = true
	}
	if decoded {
		// 标记本次有解码命中——ApplyOpenAIImageBillingResolution 会读取此 hint 设置 Source。
		result.imageSizeDecoded = true
	}
	logger.L().Info("[debug] openai.images.decode_size_on_rsp.done",
		zap.String("component", "service.openai_gateway"),
		zap.Bool("any_decoded", decoded),
		zap.Strings("image_output_sizes_final", result.ImageOutputSizes),
	)
}

// needDecodeImageSize 判定一个 size slot 是否需要触发解码：
// 空字符串或 "auto"（大小写不敏感、允许周边空白）即触发；其他值（如已有 "1024x1024"）一律跳过。
func needDecodeImageSize(size string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(size))
	return trimmed == "" || trimmed == "auto"
}

// decodeImageSizeFromBase64 解码 b64 内容头部并返回 "{w}x{h}"。
// 仅使用 image.DecodeConfig（仅读尺寸元数据，不解码像素），常见 PNG/JPEG/WebP 头部 < 1ms。
// 容错处理：
//   - 接受带 data URI 前缀（data:image/png;base64,XXX）；
//   - 输入超过上限直接 truncate 防御 OOM；
//   - 解码失败返回 error 由调用方记 warn。
func decodeImageSizeFromBase64(payload string) (string, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", fmt.Errorf("empty base64 payload")
	}
	// 去掉可能的 data URI 前缀
	if idx := strings.Index(payload, ","); idx >= 0 && strings.HasPrefix(strings.ToLower(payload), "data:") {
		payload = payload[idx+1:]
	}
	if len(payload) > imageDecodeMaxBase64Bytes {
		return "", fmt.Errorf("base64 payload too large: %d bytes (limit %d)", len(payload), imageDecodeMaxBase64Bytes)
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// 兼容 URL-safe base64 / 缺少 padding 的输入
		raw, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("base64 decode failed: %w", err)
		}
	}
	if len(raw) > imageDecodeMaxRawBytes {
		return "", fmt.Errorf("decoded payload too large: %d bytes (limit %d)", len(raw), imageDecodeMaxRawBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("image.DecodeConfig: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", fmt.Errorf("invalid image dimensions: %dx%d", cfg.Width, cfg.Height)
	}
	return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height), nil
}
