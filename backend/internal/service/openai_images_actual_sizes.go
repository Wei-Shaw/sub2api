package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAIImageSizeProbeReadBytes 是单次尺寸探测最多读取的字节数。
//
// PNG IHDR / JPEG SOF / WebP VP8X 都位于文件头部，1 MiB 足以覆盖带大型 EXIF / ICC
// 段的 JPEG；当服务端忽略 Range 直接返回整图时，读取量也由这个上限兜住。
const openAIImageSizeProbeReadBytes int64 = 1 << 20

// openAIImageSizeProbeTimeout 是单张图片尺寸探测的超时上限。
//
// 探测只取头部，正常应在毫秒级返回；超时即放弃探测并保持既有行为，
// 不因为一次探测失败而拖慢或影响生图结果。
const openAIImageSizeProbeTimeout = 5 * time.Second

// resolveOpenAIImagesResultSizes 为 Images 端点的非流式响应补齐每张图的真实交付尺寸。
//
// 背景：部分上游（尤其是订阅额度转 API 的中转）以 url 返回图片且不回显 size，导致
// usage_logs.image_output_size 为空、计费档位只能回落到请求尺寸，客户端也无法得知
// 实际交付像素。这里按以下优先级解析真实像素并写回 data[i].size：
//
//  1. 上游已给出具体尺寸（"WxH"）→ 原样保留，不覆盖上游声明；
//  2. 内联 b64_json / data URL → 解码图片头；
//  3. 远程 url → 只读取头部字节后解码（经与 url→b64_json 回填一致的出站校验）。
//
// 任何一项解析失败都只跳过该项，响应与计费照常进行（fail-soft）。写回后的
// data[i].size 同时是 collectOpenAIResponseImageOutputSizesFromJSONBytes 的唯一来源，
// 因此计费档位会跟随真实交付尺寸，而不是请求尺寸。
func (s *OpenAIGatewayService) resolveOpenAIImagesResultSizes(ctx context.Context, account *Account, body []byte) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}
	items := gjson.GetBytes(body, "data")
	if !items.IsArray() {
		return body
	}
	// 只有"由网关解析出"的尺寸才回填顶层 size，避免改变上游已声明尺寸时的响应形态。
	topLevelSize := ""
	for index, item := range items.Array() {
		if !item.IsObject() {
			continue
		}
		if isResolvedImageSize(item.Get("size").String()) {
			continue
		}
		// 内联载荷缺失时才探测远程 url：b64_json 非空说明这批字节已经取过一次
		// （上游内联返回，或 url→b64_json 回填下载过），解不出尺寸再下载同一个
		// url 也不会有不同结果，只会多一次无谓的出站请求。
		inline := strings.TrimSpace(item.Get("b64_json").String())
		size := detectOpenAIImageResultSize(inline)
		if size == "" && inline == "" {
			size = s.probeOpenAIImageURLSize(ctx, account, strings.TrimSpace(item.Get("url").String()))
		}
		if size == "" {
			continue
		}
		updated, err := sjson.SetBytes(body, fmt.Sprintf("data.%d.size", index), size)
		if err != nil {
			continue
		}
		body = updated
		if index == 0 {
			topLevelSize = size
		}
	}
	if topLevelSize != "" {
		if updated, err := sjson.SetBytes(body, "size", topLevelSize); err == nil {
			body = updated
		}
	}
	return body
}

// probeOpenAIImageURLSize 只读取远程图片的头部字节并解码出真实像素尺寸。
//
// 出站策略与 url→b64_json 回填完全一致：先按 base_url 策略校验 URL，再无条件拒绝
// 回环、私网、链路本地等目的地，并标记为只允许公网主机（重定向的每一跳同样校验），
// 经账户代理下载。与回填的差别是只请求头部（Range）且最多读取
// openAIImageSizeProbeReadBytes，避免为了读几十字节的图片头而下载整张大图。
//
// 尺寸探测是尽力而为：未启用、被拦截、超时、状态码异常、内容不是可识别的图片格式，
// 一律返回空串，由调用方保持既有行为。
func (s *OpenAIGatewayService) probeOpenAIImageURLSize(ctx context.Context, account *Account, rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "data:") {
		return detectOpenAIImageResultSize(rawURL)
	}
	if s == nil || s.httpUpstream == nil {
		return ""
	}
	downloadURL, err := s.validateOutboundURL(rawURL)
	if err != nil {
		return ""
	}
	if err := rejectPrivateImageHost(downloadURL); err != nil {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	accountConcurrency := 0
	proxyURL := ""
	if account != nil {
		accountID = account.ID
		accountConcurrency = account.Concurrency
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
	}
	probeCtx, cancel := context.WithTimeout(WithHTTPUpstreamPublicHostsOnly(ctx), openAIImageSizeProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", openAIImageSizeProbeReadBytes-1))
	resp, err := s.httpUpstream.Do(req, proxyURL, accountID, accountConcurrency)
	if err != nil {
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Images size probe skipped account_id=%d err=%s",
			accountID,
			sanitizeUpstreamErrorMessage(err.Error()),
		)
		return ""
	}
	// Do 返回 (nil, nil) 属上游客户端契约违例，这里按"探测失败"处理，
	// 绝不能因为一次尺寸探测把整个生图请求打成 panic。
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Images size probe skipped account_id=%d status=%d",
			accountID,
			resp.StatusCode,
		)
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageSizeProbeReadBytes))
	if err != nil && len(data) == 0 {
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Images size probe skipped account_id=%d err=%s",
			accountID,
			sanitizeUpstreamErrorMessage(err.Error()),
		)
		return ""
	}
	return detectOpenAIImageBytesSize(data)
}
