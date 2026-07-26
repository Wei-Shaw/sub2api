package domain

import "time"

// OpsIngressRejectAggregate 是入口拒绝聚合读模型的一行（按 bucket + 维度去重计数）。
// 属 Ops/dashboard 读模型 BC；字段均为标量 FK（UserID/APIKeyID），不嵌入其他 BC 实体。
type OpsIngressRejectAggregate struct {
	ID           int64     `json:"id"`
	BucketStart  time.Time `json:"bucket_start"`
	RejectReason string    `json:"reject_reason"`
	RouteFamily  string    `json:"route_family"`
	Protocol     string    `json:"protocol"`
	ClientIP     string    `json:"client_ip"`
	UserID       *int64    `json:"user_id,omitempty"`
	APIKeyID     *int64    `json:"api_key_id,omitempty"`
	RequestCount int64     `json:"request_count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// OpsIngressRejectFilter 过滤入口拒绝聚合读模型。
type OpsIngressRejectFilter struct {
	StartTime    *time.Time
	EndTime      *time.Time
	RejectReason string
	RouteFamily  string
	Protocol     string
	ClientIP     string
	UserID       *int64
	APIKeyID     *int64
	Page         int
	PageSize     int
}

// OpsIngressRejectList 是入口拒绝聚合的分页结果。
type OpsIngressRejectList struct {
	Items    []*OpsIngressRejectAggregate `json:"items"`
	Total    int                          `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
}
