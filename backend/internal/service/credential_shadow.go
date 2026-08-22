package service

import (
	"context"
	"fmt"
)

// resolveCredentialAccount 解析影子账号到其母账号，用于凭据/Token 透传。
// - 普通账号（非影子）：直接返回自身。
// - 影子账号：通过 repo 取母账号，校验母账号存在且为 OpenAI OAuth 类型，否则返回错误。
// 设计为包级函数（非任何 service 的方法），以便 OpenAIGatewayService / OpenAIQuotaService /
// AccountUsageService 等不同接收者共享同一实现。
func resolveCredentialAccount(ctx context.Context, repo AccountRepository, account *Account) (*Account, error) {
	if account == nil || !account.IsShadow() {
		return account, nil
	}
	parent, err := repo.GetByID(ctx, *account.ParentAccountID)
	if err != nil {
		return nil, fmt.Errorf("resolve spark shadow parent %d: %w", *account.ParentAccountID, err)
	}
	if parent == nil {
		return nil, fmt.Errorf("spark shadow parent %d not found", *account.ParentAccountID)
	}
	// 防御:创建路径已禁二级影子(G6),此处再挡一层——畸形数据/手工 DB 写出的影子→影子链
	// 会让凭据解析停在无凭据的一级影子(只解一层),fail-closed 比静默返回坏母更安全(外审第6轮)。
	if parent.IsShadow() {
		return nil, fmt.Errorf("spark shadow parent %d is itself a shadow", parent.ID)
	}
	if !parent.IsOpenAIOAuth() {
		return nil, fmt.Errorf("spark shadow parent %d is not OpenAI OAuth", parent.ID)
	}
	return parent, nil
}
