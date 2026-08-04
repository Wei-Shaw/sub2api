package service

import (
	"context"
	"errors"
)

type profitControlFailingGroupRepo struct {
	GroupRepository
}

func (profitControlFailingGroupRepo) GetByIDLite(context.Context, int64) (*Group, error) {
	return nil, errors.New("group cache unavailable")
}

// 利润控制必须走不带账号计数聚合的 lite 读取。
func (profitControlFailingGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	panic("profit control gate must read groups via GetByIDLite (no account-count aggregation)")
}
