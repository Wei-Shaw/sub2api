package domain

import (
	"encoding/json"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ImageTaskRecord 是异步图像请求在 Redis 中的私有表示（不含 owner 的公开视图字段）。
// 属 image-task BC（imgtask_ 命名空间）；字段均为标量 + json.RawMessage，无跨 BC 指针。
type ImageTaskRecord struct {
	ID          string          `json:"id"`
	UserID      int64           `json:"user_id"`
	APIKeyID    int64           `json:"api_key_id"`
	Status      string          `json:"status"`
	HTTPStatus  int             `json:"http_status,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	CreatedAt   int64           `json:"created_at"`
	CompletedAt *int64          `json:"completed_at,omitempty"`
	ExpiresAt   int64           `json:"expires_at"`
}

// ErrImageTaskNotFound indicates an async image task was not found in the store.
var ErrImageTaskNotFound = infraerrors.New(http.StatusNotFound, "IMAGE_TASK_NOT_FOUND", "image task not found")
