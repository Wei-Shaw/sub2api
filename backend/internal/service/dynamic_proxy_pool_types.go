package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrDynamicProxyPoolNotFound      = infraerrors.NotFound("DYNAMIC_PROXY_POOL_NOT_FOUND", "dynamic proxy pool not found")
	ErrDynamicProxyPoolConflict      = infraerrors.Conflict("DYNAMIC_PROXY_POOL_CONFLICT", "dynamic proxy pool name prefix conflict")
	ErrDynamicProxyPoolExtractFailed = infraerrors.BadRequest("DYNAMIC_PROXY_POOL_EXTRACT_FAILED", "failed to extract IPs from API")
)

// DynamicProxyPool represents a dynamic IP extraction pool configuration.
type DynamicProxyPool struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Enabled            bool       `json:"enabled"`
	SourceType         string     `json:"source_type"` // extract_api | subscription
	SubscriptionID     *int64     `json:"subscription_id"`
	ExtractURL         string     `json:"extract_url"`
	Protocol           string     `json:"protocol"`
	AuthMode           string     `json:"auth_mode"` // none | fixed | from_response
	Username           string     `json:"username"`
	Password           string     `json:"password"`
	ResponseFormat     string     `json:"response_format"` // txt | json
	LineSeparator      string     `json:"line_separator"`
	IPFieldPath        string     `json:"ip_field_path"`
	PortFieldPath      string     `json:"port_field_path"`
	RefreshIntervalSec int        `json:"refresh_interval_sec"`
	IPDurationSec      int        `json:"ip_duration_sec"`
	ExtractCount       int        `json:"extract_count"`
	MinAlive           int        `json:"min_alive"`
	NamePrefix         string     `json:"name_prefix"`
	LastExtractAt      *time.Time `json:"last_extract_at"`
	LastExtractStatus  string     `json:"last_extract_status"`
	LastExtractError   string     `json:"last_extract_error"`
	AliveCount              int        `json:"alive_count"`
	HealthCheckIntervalSec  int        `json:"health_check_interval_sec"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// DynamicProxyPoolRepository defines data access for dynamic proxy pools.
type DynamicProxyPoolRepository interface {
	Create(ctx context.Context, m *DynamicProxyPool) error
	GetByID(ctx context.Context, id int64) (*DynamicProxyPool, error)
	Update(ctx context.Context, m *DynamicProxyPool) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params DynamicProxyPoolListParams) ([]*DynamicProxyPool, int64, error)
	ListEnabled(ctx context.Context) ([]*DynamicProxyPool, error)
	ExistsNamePrefix(ctx context.Context, prefix string, excludeID int64) (bool, error)
	UpdateExtractState(ctx context.Context, id int64, status, errMsg string, lastExtractAt *time.Time) error
	UpdateAliveCount(ctx context.Context, id int64, count int) error
}

// DynamicProxyPoolListParams controls listing and pagination.
type DynamicProxyPoolListParams struct {
	Page     int
	PageSize int
	Search   string
	Enabled  *bool
}

// DynamicProxyPoolCreateParams is the input for creating a pool.
type DynamicProxyPoolCreateParams struct {
	Name               string `json:"name"`
	SourceType         string `json:"source_type"`
	SubscriptionID     *int64 `json:"subscription_id"`
	ExtractURL         string `json:"extract_url"`
	Protocol           string `json:"protocol"`
	AuthMode           string `json:"auth_mode"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	ResponseFormat     string `json:"response_format"`
	LineSeparator      string `json:"line_separator"`
	IPFieldPath        string `json:"ip_field_path"`
	PortFieldPath      string `json:"port_field_path"`
	RefreshIntervalSec int    `json:"refresh_interval_sec"`
	IPDurationSec      int    `json:"ip_duration_sec"`
	ExtractCount       int    `json:"extract_count"`
	MinAlive           int    `json:"min_alive"`
	HealthCheckIntervalSec int `json:"health_check_interval_sec"`
}

// DynamicProxyPoolUpdateParams is the input for updating a pool.
type DynamicProxyPoolUpdateParams struct {
	Name               *string `json:"name"`
	Enabled            *bool   `json:"enabled"`
	SourceType         *string `json:"source_type"`
	SubscriptionID     *int64  `json:"subscription_id"`
	ExtractURL         *string `json:"extract_url"`
	Protocol           *string `json:"protocol"`
	AuthMode           *string `json:"auth_mode"`
	Username           *string `json:"username"`
	Password           *string `json:"password"`
	ResponseFormat     *string `json:"response_format"`
	LineSeparator      *string `json:"line_separator"`
	IPFieldPath        *string `json:"ip_field_path"`
	PortFieldPath      *string `json:"port_field_path"`
	RefreshIntervalSec *int    `json:"refresh_interval_sec"`
	IPDurationSec      *int    `json:"ip_duration_sec"`
	ExtractCount       *int    `json:"extract_count"`
	MinAlive           *int    `json:"min_alive"`
	HealthCheckIntervalSec *int `json:"health_check_interval_sec"`
}

// DynamicProxyPoolExtractResult summarizes one extraction run.
type DynamicProxyPoolExtractResult struct {
	Created    int `json:"created"`
	Failed     int `json:"failed"`
	AliveCount int `json:"alive_count"`
}
