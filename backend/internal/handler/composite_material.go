package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/rpc/innerpb"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"trpc.group/trpc-go/trpc-go/client"
)

const defaultCompositeMaterialMaxBytes int64 = 50 * 1024 * 1024

type compositeMaterialClient interface {
	UploadMaterial(ctx context.Context, req *innerpb.UploadMaterialRequest, opts ...client.Option) (*innerpb.UploadMaterialResponse, error)
}

var newCompositeMaterialClient = func(target string) compositeMaterialClient {
	return innerpb.NewInnerAPIClientProxy(client.WithTarget(target))
}

func (h *ModelAPIGatewayHandler) prepareCompositeMaterialPayload(ctx context.Context, apiKey *service.APIKey, payload map[string]any) (map[string]any, error) {
	needsUpload := false
	if raw, ok := payload["image_urls"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return payload, errors.New("image_urls must be an array")
		}
		for idx, item := range items {
			value, ok := item.(string)
			if !ok {
				return payload, fmt.Errorf("image_urls[%d] must be a string", idx)
			}
			value = strings.TrimSpace(value)
			if strings.HasPrefix(strings.ToLower(value), "data:") {
				needsUpload = true
				continue
			}
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return payload, fmt.Errorf("image_urls[%d] must be a data URL or an HTTP URL", idx)
			}
		}
	}
	if raw, ok := payload["mask_url"]; ok {
		value, ok := raw.(string)
		if !ok {
			return payload, errors.New("mask_url must be a string")
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(value), "data:") {
			needsUpload = true
		} else if value != "" {
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return payload, errors.New("mask_url must be a data URL or an HTTP URL")
			}
		}
	}
	if !needsUpload {
		return payload, nil
	}
	if h.cfg == nil || !h.cfg.CompositeMaterial.Enabled {
		return payload, errors.New("composite material upload is not configured")
	}
	settings := h.cfg.CompositeMaterial
	if strings.TrimSpace(settings.Host) == "" || settings.Port <= 0 || strings.TrimSpace(settings.AppID) == "" || strings.TrimSpace(settings.Token) == "" {
		return payload, errors.New("composite material upload configuration is incomplete")
	}
	accountID := strings.TrimSpace(settings.AccountID)
	if accountID == "" && apiKey != nil && apiKey.User != nil {
		accountID = strings.TrimSpace(apiKey.User.AccountID)
	}
	if accountID == "" {
		return payload, errors.New("composite material account_id is missing")
	}
	maxBytes := settings.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultCompositeMaterialMaxBytes
	}
	proxy := newCompositeMaterialClient(fmt.Sprintf("ip://%s:%d", strings.TrimSpace(settings.Host), settings.Port))
	upload := func(value, fileName string) (string, error) {
		data, contentType, err := decodeCompositeDataURL(value, maxBytes)
		if err != nil {
			return "", err
		}
		fileName += compositeMaterialExtension(contentType)
		response, err := proxy.UploadMaterial(ctx, &innerpb.UploadMaterialRequest{
			AccountId:   accountID,
			FileName:    fileName,
			ContentType: contentType,
			Data:        data,
		}, client.WithMetaData("app-token", []byte(settings.Token)))
		if err != nil {
			return "", fmt.Errorf("upload %s: %w", fileName, err)
		}
		if response == nil {
			return "", fmt.Errorf("upload %s: empty response", fileName)
		}
		fileURL := strings.TrimSpace(response.GetFileUrl())
		if fileURL == "" && response.GetMaterial() != nil {
			fileURL = strings.TrimSpace(response.GetMaterial().GetUrl())
		}
		if fileURL == "" {
			return "", fmt.Errorf("upload %s: response did not include a URL", fileName)
		}
		return fileURL, nil
	}

	if raw, ok := payload["image_urls"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return payload, errors.New("image_urls must be an array")
		}
		for idx, item := range items {
			value, ok := item.(string)
			if !ok {
				return payload, fmt.Errorf("image_urls[%d] must be a string", idx)
			}
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:") {
				continue
			}
			fileURL, err := upload(value, fmt.Sprintf("composite-reference-%d", idx+1))
			if err != nil {
				return payload, err
			}
			items[idx] = fileURL
		}
		payload["image_urls"] = items
	}
	if raw, ok := payload["mask_url"]; ok {
		value, ok := raw.(string)
		if !ok {
			return payload, errors.New("mask_url must be a string")
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:") {
			fileURL, err := upload(value, "composite-mask")
			if err != nil {
				return payload, err
			}
			payload["mask_url"] = fileURL
		}
	}
	return payload, nil
}

func compositeMaterialExtension(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func decodeCompositeDataURL(value string, maxBytes int64) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return nil, "", errors.New("material reference must be a data URL or an HTTP URL")
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return nil, "", errors.New("invalid material data URL")
	}
	header := value[5:comma]
	parts := strings.Split(header, ";")
	contentType := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", errors.New("material data URL must contain an image")
	}
	encoded := value[comma+1:]
	var data []byte
	var err error
	if len(parts) > 1 && strings.EqualFold(parts[len(parts)-1], "base64") {
		data, err = base64.StdEncoding.DecodeString(encoded)
	} else {
		decoded, decodeErr := url.PathUnescape(encoded)
		if decodeErr != nil {
			return nil, "", fmt.Errorf("decode material data URL: %w", decodeErr)
		}
		data = []byte(decoded)
	}
	if err != nil {
		return nil, "", fmt.Errorf("decode material data URL: %w", err)
	}
	if len(data) == 0 {
		return nil, "", errors.New("material data URL is empty")
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("material exceeds configured size limit of %d bytes", maxBytes)
	}
	return data, contentType, nil
}
