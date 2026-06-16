// Package admin: account dedup identity helpers.
//
// 账号去重的统一身份键计算。
// 不同平台/类型用不同的稳定字段做身份依据：
//   - OpenAI OAuth: chatgpt_account_id / chatgpt_user_id / email
//   - Claude OAuth/SetupToken: account_uuid (来自 extra) / access_token 指纹
//   - Gemini OAuth / Antigravity OAuth: email / access_token 指纹
//   - 任意平台的 api_key / upstream / bedrock(api_key) / anthropic_aws: api_key 指纹 + base_url
//   - Gemini service_account: client_email
//   - Bedrock SigV4: aws_access_key_id + region
//
// 所有键都带 "platform:type:" 前缀，避免跨平台/跨类型误判。
package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// accountIdentityInput 构建身份键所需的最小输入。
type accountIdentityInput struct {
	Platform    string
	Type        string
	Credentials map[string]any
	Extra       map[string]any
}

// accountIdentityIndex 维护 identityKey → 已存在账号的映射，用于在创建/导入时检测重复。
type accountIdentityIndex struct {
	byKey map[string]service.Account
}

func newAccountIdentityIndex() *accountIdentityIndex {
	return &accountIdentityIndex{byKey: map[string]service.Account{}}
}

// Add 将一个账号的所有身份键写入索引；同一账号可能产生多个键。
func (i *accountIdentityIndex) Add(account service.Account) {
	if i == nil {
		return
	}
	if i.byKey == nil {
		i.byKey = map[string]service.Account{}
	}
	keys := buildAccountIdentityKeysFromAccount(account)
	for _, key := range keys {
		i.byKey[key] = account
	}
}

// Find 按身份键集合查找已存在账号；任一键命中即返回。
func (i *accountIdentityIndex) Find(keys []string) *service.Account {
	if i == nil {
		return nil
	}
	for _, key := range keys {
		if account, ok := i.byKey[key]; ok {
			return &account
		}
	}
	return nil
}

// buildAccountIdentityKeysFromAccount 从已存在的 service.Account 提取身份键。
func buildAccountIdentityKeysFromAccount(account service.Account) []string {
	return buildAccountIdentityKeys(accountIdentityInput{
		Platform:    account.Platform,
		Type:        account.Type,
		Credentials: account.Credentials,
		Extra:       account.Extra,
	})
}

// buildAccountIdentityKeys 根据平台/类型/凭证生成稳定身份键。
//
// 同一账号可能产生多个键（如 OpenAI OAuth 同时有 account_id / user_id / email）。
// 任一键命中即认为重复。返回的键集合可能为空，调用方需自行判定（空集表示无法去重）。
func buildAccountIdentityKeys(input accountIdentityInput) []string {
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	accType := strings.ToLower(strings.TrimSpace(input.Type))
	if platform == "" || accType == "" {
		return nil
	}
	prefix := platform + ":" + accType + ":"
	keys := make([]string, 0, 4)

	switch platform {
	case service.PlatformOpenAI:
		if accType == service.AccountTypeOAuth {
			keys = appendOAuthOpenAIKeys(keys, prefix, input.Credentials)
		} else {
			keys = appendAPIKeyKeys(keys, prefix, input.Credentials)
		}
	case service.PlatformAnthropic:
		switch accType {
		case service.AccountTypeOAuth, service.AccountTypeSetupToken:
			keys = appendOAuthClaudeKeys(keys, prefix, input.Credentials, input.Extra)
		case service.AccountTypeBedrock:
			keys = appendBedrockKeys(keys, prefix, input.Credentials)
		default:
			// apikey / upstream / anthropic_aws
			keys = appendAPIKeyKeys(keys, prefix, input.Credentials)
		}
	case service.PlatformGemini:
		switch accType {
		case service.AccountTypeOAuth:
			keys = appendOAuthCommonKeys(keys, prefix, input.Credentials)
		case service.AccountTypeServiceAccount:
			keys = appendServiceAccountKeys(keys, prefix, input.Credentials)
		default:
			keys = appendAPIKeyKeys(keys, prefix, input.Credentials)
		}
	case service.PlatformAntigravity:
		if accType == service.AccountTypeOAuth {
			keys = appendOAuthCommonKeys(keys, prefix, input.Credentials)
		} else {
			keys = appendAPIKeyKeys(keys, prefix, input.Credentials)
		}
	default:
		// 未知平台兜底：按 api_key 指纹 + base_url 处理。
		keys = appendAPIKeyKeys(keys, prefix, input.Credentials)
	}
	return keys
}

func appendOAuthOpenAIKeys(keys []string, prefix string, credentials map[string]any) []string {
	accountID := credentialString(credentials, "chatgpt_account_id")
	userID := credentialString(credentials, "chatgpt_user_id")
	email := strings.ToLower(credentialString(credentials, "email"))
	accessToken := credentialString(credentials, "access_token")
	if accountID != "" {
		keys = append(keys, prefix+"account:"+accountID)
	}
	if userID != "" {
		keys = append(keys, prefix+"user:"+userID)
	}
	if accountID == "" && userID == "" && email != "" {
		keys = append(keys, prefix+"email:"+email)
	}
	if accessToken != "" {
		keys = append(keys, prefix+"access:"+tokenFingerprint(accessToken))
	}
	return keys
}

func appendOAuthClaudeKeys(keys []string, prefix string, credentials, extra map[string]any) []string {
	accountUUID := extraString(extra, "account_uuid")
	orgUUID := extraString(extra, "org_uuid")
	accessToken := credentialString(credentials, "access_token")
	if accountUUID != "" {
		keys = append(keys, prefix+"account_uuid:"+accountUUID)
	}
	if orgUUID != "" && accountUUID == "" {
		// org_uuid 单独不足以唯一定位账号，仅在没有 account_uuid 时作为兜底。
		keys = append(keys, prefix+"org_uuid:"+orgUUID)
	}
	if accessToken != "" {
		keys = append(keys, prefix+"access:"+tokenFingerprint(accessToken))
	}
	return keys
}

func appendOAuthCommonKeys(keys []string, prefix string, credentials map[string]any) []string {
	email := strings.ToLower(credentialString(credentials, "email"))
	accessToken := credentialString(credentials, "access_token")
	if email != "" {
		keys = append(keys, prefix+"email:"+email)
	}
	if accessToken != "" {
		keys = append(keys, prefix+"access:"+tokenFingerprint(accessToken))
	}
	return keys
}

func appendAPIKeyKeys(keys []string, prefix string, credentials map[string]any) []string {
	apiKey := credentialString(credentials, "api_key")
	if apiKey == "" {
		// 部分平台/类型字段名差异
		apiKey = credentialString(credentials, "key")
	}
	baseURL := strings.ToLower(credentialString(credentials, "base_url"))
	if apiKey != "" {
		suffix := tokenFingerprint(apiKey)
		if baseURL != "" {
			suffix = baseURL + "|" + suffix
		}
		keys = append(keys, prefix+"apikey:"+suffix)
	}
	return keys
}

func appendBedrockKeys(keys []string, prefix string, credentials map[string]any) []string {
	authMode := strings.ToLower(credentialString(credentials, "auth_mode"))
	if authMode == "sigv4" || authMode == "" {
		akid := credentialString(credentials, "aws_access_key_id")
		region := strings.ToLower(credentialString(credentials, "aws_region"))
		if akid != "" {
			key := prefix + "aws_akid:" + akid
			if region != "" {
				key += "|" + region
			}
			keys = append(keys, key)
			return keys
		}
	}
	// 回退到 api_key 模式
	return appendAPIKeyKeys(keys, prefix, credentials)
}

func appendServiceAccountKeys(keys []string, prefix string, credentials map[string]any) []string {
	clientEmail := strings.ToLower(credentialString(credentials, "client_email"))
	if clientEmail != "" {
		keys = append(keys, prefix+"client_email:"+clientEmail)
	}
	return keys
}

// credentialString 兼容 service.Account.GetCredential 的取值规则，但接受裸 map。
func credentialString(credentials map[string]any, key string) string {
	if credentials == nil {
		return ""
	}
	return strings.TrimSpace(codexStringValue(credentials[key]))
}

// extraString 取 extra map 中的字符串字段。
func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	return strings.TrimSpace(codexStringValue(extra[key]))
}

// tokenFingerprint 计算 token 的 SHA256 指纹，避免直接持有/比较 token 明文。
func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// firstSeenIdentity 检查批内是否已有相同身份键，返回首次出现的索引。
func firstSeenIdentity(seen map[string]int, keys []string) (int, bool) {
	for _, key := range keys {
		if index, ok := seen[key]; ok {
			return index, true
		}
	}
	return 0, false
}

// markIdentitySeen 记录一组身份键到批内已见集合。
func markIdentitySeen(seen map[string]int, keys []string, index int) {
	for _, key := range keys {
		seen[key] = index
	}
}

// loadAccountIdentityIndex 加载现有账号并按指定平台集构建身份索引。
// 当 platforms 为空时不加载任何数据；调用方应明确传入待去重的平台集合，
// 利用 (platform) 索引避免全表扫描。
func (h *AccountHandler) loadAccountIdentityIndex(ctx context.Context, platforms []string) (*accountIdentityIndex, error) {
	index := newAccountIdentityIndex()
	seen := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform == "" {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		accounts, err := h.listAccountsFiltered(ctx, platform, "", "", "", 0, "", "created_at", "desc")
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			index.Add(account)
		}
	}
	return index, nil
}

// findDuplicateAccount 查询数据库中是否已有相同身份的账号。
// 当输入身份键为空（即无法基于凭证生成身份），返回 (nil, nil)，调用方按非重复处理。
func (h *AccountHandler) findDuplicateAccount(ctx context.Context, input accountIdentityInput) (*service.Account, error) {
	keys := buildAccountIdentityKeys(input)
	if len(keys) == 0 {
		return nil, nil
	}
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	accounts, err := h.listAccountsFiltered(ctx, platform, "", "", "", 0, "", "created_at", "desc")
	if err != nil {
		return nil, err
	}
	index := newAccountIdentityIndex()
	for _, account := range accounts {
		index.Add(account)
	}
	return index.Find(keys), nil
}
