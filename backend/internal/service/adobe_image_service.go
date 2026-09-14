package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/google/uuid"
)

// AdobeImageService 把一次 OpenAI 形状的出图请求翻译成 Adobe Firefly 直连调用，
// 再把产物翻译回 OpenAI 形状的响应。
//
// 它不做账号选择与 failover——那是 handler 的职责；本服务只负责「给定账号和 token
// 完成一次出图」，因此可以脱离 gin 与调度器单测。
type AdobeImageService struct {
	clients *adobeClientCache
	// resolveStorage 为 nil 或返回未启用时，响应回落 b64_json。
	resolveStorage ImageStorageResolver
	// inputClient 抓取图生图的源图；见 adobe_image_input.go 的 SSRF 说明。
	inputClient *http.Client
}

func NewAdobeImageService(resolveStorage ImageStorageResolver) *AdobeImageService {
	return &AdobeImageService{
		clients:        &adobeClientCache{},
		resolveStorage: resolveStorage,
		inputClient:    newAdobeInputImageClient(),
	}
}

// AdobeImageResult 是一次出图的完整产出。
type AdobeImageResult struct {
	// Body 是可直接写给客户端的 OpenAI 形状响应。
	Body []byte
	// Forward 交给 RecordUsage 记账；计费只认 ImageCount 与 ImageSize 两个字段。
	Forward *OpenAIForwardResult
}

// Generate 用指定账号完成一次出图。
//
// 错误一律是 adobe 包的类型化错误或本包的 *adobe.RequestError（终态），
// 交由 classifyAdobeError 翻译成网关语义。
func (s *AdobeImageService) Generate(
	ctx context.Context, account *Account, token string, req *OpenAIImagesRequest,
) (*AdobeImageResult, error) {
	if account == nil {
		return nil, adobe.NewRequestError("adobe account is required")
	}
	if req == nil {
		return nil, adobe.NewRequestError("images request is required")
	}
	// Adobe 单次只出一张。静默只返回一张会让客户端以为 n 生效了，
	// 明确报错比让用户对着账单猜好。
	if req.N > 1 {
		return nil, adobe.NewRequestError(
			fmt.Sprintf("adobe channel generates one image per request, got n=%d", req.N))
	}
	if strings.TrimSpace(token) == "" {
		return nil, adobe.NewAuthError("adobe access token is empty", http.StatusUnauthorized)
	}

	requestedModel := strings.TrimSpace(req.Model)
	upstreamModelID := account.GetMappedModel(requestedModel)
	conf, err := adobe.ResolveImage(adobe.ImageRequest{ModelID: upstreamModelID, Size: req.Size})
	if err != nil {
		return nil, err
	}

	client := s.clients.clientForAccount(account)
	sourceImageIDs, err := s.uploadSourceImages(ctx, client, token, req)
	if err != nil {
		return nil, err
	}

	generated, err := client.GenerateImage(ctx, adobe.GenerateImageInput{
		Token: token,
		Options: adobe.ImagePayloadOptions{
			Prompt:               req.Prompt,
			AspectRatio:          conf.AspectRatio,
			OutputResolution:     conf.OutputResolution,
			UpstreamModelID:      conf.UpstreamModelID,
			UpstreamModelVersion: conf.UpstreamModelVersion,
			PayloadKind:          conf.PayloadKind,
			SizePixels:           conf.SizePixels,
			QualityLevel:         req.Quality,
			SourceImageIDs:       sourceImageIDs,
			Background:           req.Background,
		},
	})
	if err != nil {
		return nil, err
	}

	requestID := uuid.NewString()
	body, err := s.buildResponseBody(ctx, requestID, generated.Bytes)
	if err != nil {
		return nil, err
	}

	return &AdobeImageResult{
		Body: body,
		Forward: &OpenAIForwardResult{
			RequestID:  requestID,
			Model:      requestedModel,
			ImageCount: 1,
			// 计费档位：adobe 的 OutputResolution 取值就是 "1K"/"2K"/"4K"，
			// ClassifyImageBillingTier 直接认这三个字面量。
			ImageSize:        string(conf.OutputResolution),
			UpstreamModel:    conf.ModelID,
			UpstreamEndpoint: adobe.ImageSubmitURL,
		},
	}, nil
}

// uploadSourceImages 把图生图的源图上传到 Adobe，返回可放进 payload 的 image id。
// 文生图时返回 nil。
func (s *AdobeImageService) uploadSourceImages(
	ctx context.Context, client *adobe.Client, token string, req *OpenAIImagesRequest,
) ([]string, error) {
	if len(req.Uploads) == 0 && len(req.InputImageURLs) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(req.Uploads)+len(req.InputImageURLs))
	// multipart 上传的图已经是现成字节，直接转发。
	for _, upload := range req.Uploads {
		if len(upload.Data) == 0 {
			continue
		}
		id, err := client.UploadImage(ctx, token, upload.Data, upload.ContentType)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	// JSON 体里的 URL 需要先取回字节。
	for _, rawURL := range req.InputImageURLs {
		image, err := fetchAdobeInputImage(ctx, s.inputClient, rawURL)
		if err != nil {
			// 取图失败是请求本身的问题，换账号也救不了。
			return nil, adobe.NewRequestError(err.Error())
		}
		id, err := client.UploadImage(ctx, token, image.Data, image.ContentType)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

// buildResponseBody 构造 OpenAI 形状的响应。
//
// 先按 b64_json 组装，再在对象存储可用时整体过一遍 ImageResultUploader.Rewrite——
// 它会把每项的 b64_json 上传后替换成 url。这样两条分支共用同一段组装逻辑。
func (s *AdobeImageService) buildResponseBody(ctx context.Context, requestID string, image []byte) ([]byte, error) {
	if len(image) == 0 {
		return nil, adobe.NewRequestError("adobe returned an empty image")
	}
	payload := map[string]any{
		"created": time.Now().Unix(),
		"data": []any{
			map[string]any{"b64_json": base64.StdEncoding.EncodeToString(image)},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, adobe.NewRequestError(fmt.Sprintf("encode images response: %v", err))
	}

	if s.resolveStorage == nil {
		return body, nil
	}
	uploader, enabled := s.resolveStorage()
	if !enabled || uploader == nil {
		return body, nil
	}
	rewritten, err := uploader.Rewrite(ctx, requestID, body)
	if err != nil {
		// 转存失败不该让已经生成好（且已扣上游额度）的图丢掉，回落 b64_json。
		return body, nil //nolint:nilerr // 有意吞掉：产物已生成，降级返回优于整体失败
	}
	return rewritten, nil
}
