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

// AnthropicFileOwnerDecision 是 Anthropic Messages 请求中 file 源归属的判定结果。
// 解析顺序保证拒绝优先：storeError > foreign > unknown > conflict > pinned > none。
type AnthropicFileOwnerDecision int

const (
	// AnthropicFileOwnerNone 表示请求体没有任何 file 源，走原选号路径。
	AnthropicFileOwnerNone AnthropicFileOwnerDecision = iota
	// AnthropicFileOwnerPinned 表示全部 file 源都命中、属于请求用户，且指向同一
	// 账号+平台，可硬钉回上传账号。
	AnthropicFileOwnerPinned
	// AnthropicFileOwnerForeign 表示任一 file 源已知属于其他用户，必须拒绝（403）。
	AnthropicFileOwnerForeign
	// AnthropicFileOwnerUnknown 表示存在未命中/旧格式/字段非法的 file 源，或无法
	// 识别请求用户（userID<=0），无法证明归属，必须拒绝并要求重新上传（400）。
	AnthropicFileOwnerUnknown
	// AnthropicFileOwnerConflict 表示 file 源都属于请求用户但指向不同账号，一个
	// 请求无法同时满足，必须拒绝（400）。
	AnthropicFileOwnerConflict
	// AnthropicFileOwnerStoreError 表示归属缓存读取失败，无法判定，返回可重试
	// 错误（503）。
	AnthropicFileOwnerStoreError
)

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
// 归属（归属用户 + 上传账号）。只接受 type=="file" 且 id 非空的响应；解析或
// 写入失败都只记日志，不影响已经成功的上传响应。
func (s *OpenAIGatewayService) recordAnthropicFileOwner(ctx context.Context, userID int64, account *Account, groupID *int64, respBody []byte) {
	if len(respBody) == 0 || gjson.GetBytes(respBody, "type").String() != "file" {
		return
	}
	fileID := strings.TrimSpace(gjson.GetBytes(respBody, "id").String())
	if fileID == "" {
		return
	}
	if err := s.BindAnthropicFileOwner(ctx, userID, account, fileID); err != nil {
		accountID := int64(0)
		if account != nil {
			accountID = account.ID
		}
		slog.Warn("anthropic_file_owner_bind_failed",
			"user_id", userID,
			"group_id", derefGroupID(groupID),
			"account_id", accountID,
			"error", err,
		)
	}
}

// BindAnthropicFileOwner 记录 Anthropic 形状 file_id 由用户 ownerUserID 在
// account 上传。ownerUserID<=0 表示无法确定归属用户：直接返回 nil 且不写缓存，
// 之后请求带该 file_id 时无法证明归属，会被判定为 unknown 并要求重新上传
// （fail-closed），绝不会被硬钉到某个账号。上传响应已写回客户端前调用；
// 调用方失败只记日志，不影响上传结果。
func (s *OpenAIGatewayService) BindAnthropicFileOwner(ctx context.Context, ownerUserID int64, account *Account, fileID string) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("anthropic file owner cache is unavailable")
	}
	if ownerUserID <= 0 {
		return nil
	}
	if account == nil || account.ID <= 0 {
		return fmt.Errorf("anthropic file owner account is invalid")
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return fmt.Errorf("anthropic file id is empty")
	}
	return s.cache.SetAnthropicFileOwner(ctx, fileID, ownerUserID, account.ID, account.Platform, anthropicFileOwnerTTL)
}

// ResolveAnthropicFileOwner 解析请求体里的 file 源并查缓存归属，返回明确判定：
//   - AnthropicFileOwnerNone：请求体没有 file 源，走原选号路径；
//   - AnthropicFileOwnerPinned：全部 file 源命中、属于 requesterUserID 且指向
//     同一账号+平台，可硬钉；
//   - AnthropicFileOwnerForeign：任一命中记录 ownerUserID != requesterUserID，
//     调用方必须 403；
//   - AnthropicFileOwnerUnknown：任一 file 源未命中/旧格式/字段非法，或
//     requesterUserID<=0，调用方必须 400 要求重新上传；
//   - AnthropicFileOwnerConflict：都属于 requesterUserID 但指向不同账号，
//     调用方必须 400；
//   - AnthropicFileOwnerStoreError：归属缓存读取失败（err 非 nil），调用方
//     应返回 503 可重试错误。
//
// 判定优先级固定为 storeError > foreign > unknown > conflict > pinned > none：
// 拒绝优先，且绝不把未知/读失败的 file 源当作通过后硬钉——否则自有已绑定
// file 与未知 file 混合时会把未授权文件一起转发。缓存读取错误按未知处理，
// 不阻断为"无归属"。
func (s *OpenAIGatewayService) ResolveAnthropicFileOwner(ctx context.Context, requesterUserID int64, body []byte) (ownerAccountID int64, platform string, decision AnthropicFileOwnerDecision, err error) {
	fileIDs := AnthropicFileIDs(body)
	if len(fileIDs) == 0 {
		return 0, "", AnthropicFileOwnerNone, nil
	}
	if s == nil || s.cache == nil {
		return 0, "", AnthropicFileOwnerStoreError, fmt.Errorf("anthropic file owner cache is unavailable")
	}
	if requesterUserID <= 0 {
		return 0, "", AnthropicFileOwnerUnknown, nil
	}
	ownerID := int64(0)
	ownerPlatform := ""
	hit := false
	conflict := false
	foreign := false
	unknown := false
	var storeErr error
	for _, fileID := range fileIDs {
		cachedUserID, accountID, cachedPlatform, found, err := s.cache.GetAnthropicFileOwner(ctx, fileID)
		if err != nil {
			slog.Warn("anthropic_file_owner_lookup_failed", "error", err)
			if storeErr == nil {
				storeErr = err
			}
			continue
		}
		if !found || cachedUserID <= 0 || accountID <= 0 || cachedPlatform == "" {
			unknown = true
			continue
		}
		if cachedUserID != requesterUserID {
			foreign = true
			continue
		}
		if !hit {
			ownerID, ownerPlatform, hit = accountID, cachedPlatform, true
			continue
		}
		if accountID != ownerID || cachedPlatform != ownerPlatform {
			conflict = true
		}
	}
	switch {
	case storeErr != nil:
		return 0, "", AnthropicFileOwnerStoreError, storeErr
	case foreign:
		return 0, "", AnthropicFileOwnerForeign, nil
	case unknown:
		return 0, "", AnthropicFileOwnerUnknown, nil
	case conflict:
		return 0, "", AnthropicFileOwnerConflict, nil
	case hit:
		return ownerID, ownerPlatform, AnthropicFileOwnerPinned, nil
	default:
		return 0, "", AnthropicFileOwnerUnknown, nil
	}
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
