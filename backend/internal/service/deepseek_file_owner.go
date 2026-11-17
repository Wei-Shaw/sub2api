package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// anthropicFileOwnerTTL 是 file_id → 上传账号归属的缓存有效期，固定 7 天，
// 对齐 DeepSeek/DSH 默认文件有效期。已知简化：未从上传请求的 expires_after
// 解析实际有效期，客户端显式要求更长/更短都会按 7 天处理。
const anthropicFileOwnerTTL = 7 * 24 * time.Hour

// AnthropicFileIDs 扫描 Anthropic Messages 请求体，按出现顺序返回去重后的
// file 源 file_id。支持两种位置：
//   - messages[].content[] 中 {"type":"image"|"document","source":{"type":"file","file_id":"..."}}
//   - messages[].content[].type=="tool_result" 的 .content[] 中同样形状的块
//
// base64 图片/文本源、普通工具调用都不会产生 file_id，故返回空切片。
func AnthropicFileIDs(body []byte) []string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return nil
	}
	var ids []string
	seen := make(map[string]struct{})

	appendBlock := func(block gjson.Result) {
		blockType := block.Get("type").String()
		if blockType != "image" && blockType != "document" {
			return
		}
		source := block.Get("source")
		if source.Get("type").String() != "file" {
			return
		}
		fileID := strings.TrimSpace(source.Get("file_id").String())
		if fileID == "" {
			return
		}
		if _, ok := seen[fileID]; ok {
			return
		}
		seen[fileID] = struct{}{}
		ids = append(ids, fileID)
	}

	messages.ForEach(func(_, message gjson.Result) bool {
		message.Get("content").ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").String() == "tool_result" {
				block.Get("content").ForEach(func(_, nested gjson.Result) bool {
					appendBlock(nested)
					return true
				})
				return true
			}
			appendBlock(block)
			return true
		})
		return true
	})
	return ids
}

// recordAnthropicFileOwner 从 Anthropic Files 上传响应中提取 file_id 并记录
// 归属。只接受 type=="file" 且 id 非空的响应；解析或写入失败都只记日志，
// 不影响已经成功的上传响应。
func (s *OpenAIGatewayService) recordAnthropicFileOwner(ctx context.Context, account *Account, groupID *int64, respBody []byte) {
	if len(respBody) == 0 || gjson.GetBytes(respBody, "type").String() != "file" {
		return
	}
	fileID := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if fileID == "" {
		return
	}
	if err := s.BindAnthropicFileOwner(ctx, account, fileID); err != nil {
		accountID := int64(0)
		if account != nil {
			accountID = account.ID
		}
		slog.Warn("anthropic_file_owner_bind_failed",
			"group_id", derefGroupID(groupID),
			"account_id", accountID,
			"error", err,
		)
	}
}

// BindAnthropicFileOwner 记录 Anthropic 形状 file_id 由 account 上传。上传响应
// 已写回客户端前调用；调用方失败只记日志，不影响上传结果。
func (s *OpenAIGatewayService) BindAnthropicFileOwner(ctx context.Context, account *Account, fileID string) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("anthropic file owner cache is unavailable")
	}
	if account == nil || account.ID <= 0 {
		return fmt.Errorf("anthropic file owner account is invalid")
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return fmt.Errorf("anthropic file id is empty")
	}
	return s.cache.SetAnthropicFileOwner(ctx, fileID, account.ID, account.Platform, anthropicFileOwnerTTL)
}

// ResolveAnthropicFileOwner 解析请求体里的 file 源，并查缓存得到上传账号。
// 只有"至少一个 file_id 命中，且所有命中的 file_id 都指向同一账号+平台"时
// 才返回 ok=true；否则退回原选号路径（命中不一致或全部未命中都返回 false）。
// 缓存读取错误按未命中处理并记日志：归属记录不可用时不应阻断正常调度。
func (s *OpenAIGatewayService) ResolveAnthropicFileOwner(ctx context.Context, body []byte) (int64, string, bool) {
	if s == nil || s.cache == nil {
		return 0, "", false
	}
	fileIDs := AnthropicFileIDs(body)
	if len(fileIDs) == 0 {
		return 0, "", false
	}
	ownerID := int64(0)
	ownerPlatform := ""
	hit := false
	for _, fileID := range fileIDs {
		accountID, platform, ok, err := s.cache.GetAnthropicFileOwner(ctx, fileID)
		if err != nil {
			slog.Warn("anthropic_file_owner_lookup_failed", "error", err)
			continue
		}
		if !ok || accountID <= 0 || platform == "" {
			continue
		}
		if !hit {
			ownerID, ownerPlatform, hit = accountID, platform, true
			continue
		}
		if accountID != ownerID || platform != ownerPlatform {
			// 多个 file 指向不同上游：不存在能同时满足它们的账号，放弃硬钉。
			return 0, "", false
		}
	}
	if !hit {
		return 0, "", false
	}
	return ownerID, ownerPlatform, true
}

// SelectAnthropicFileOwnerAccount 只准入 file_id 的上传账号：复用 Grok video
// 的同款硬钉路径（PreserveStickyBinding + DisableStickyEscape），失败即
// ErrNoAvailableAccounts，绝不回退到组内其它上游（换号只会继续 400）。
func (s *OpenAIGatewayService) SelectAnthropicFileOwnerAccount(
	ctx context.Context, groupID *int64, ownerID int64, platform, requestedModel string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerSessionSticky}
	platform = strings.TrimSpace(platform)
	if ownerID <= 0 || platform == "" {
		return nil, decision, ErrNoAvailableAccounts
	}
	// sessionHash 仅用于满足 selectBySessionHash 的非空校验；StickyAccountID 已
	// 给出且 PreserveStickyBinding=true，该键既不会被读取也不会被改写。
	sessionHash := "anthropic-file-owner:" + NormalizeOpenAICompatiblePlatform(platform) + ":" + strconv.FormatInt(ownerID, 10)
	return s.SelectMediaVideoRequestAccount(ctx, groupID, sessionHash, ownerID, requestedModel, platform)
}
