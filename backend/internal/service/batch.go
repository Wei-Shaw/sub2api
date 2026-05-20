package service

import (
	"context"
	"errors"
	"time"
)

var ErrBatchNotFound = errors.New("batch not found")

// Batch 批次实体
type Batch struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Source       string    `json:"source"` // codex_import, apikey_import, manual
	AccountCount int64     `json:"account_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BatchRepository 批次数据访问接口
type BatchRepository interface {
	Create(ctx context.Context, batch *Batch) error
	GetByID(ctx context.Context, id int64) (*Batch, error)
	GetByName(ctx context.Context, name string) (*Batch, error)
	List(ctx context.Context) ([]Batch, error)
	Update(ctx context.Context, batch *Batch) error
	Delete(ctx context.Context, id int64) error
	UpdateAccountCount(ctx context.Context, batchID int64) error
}
