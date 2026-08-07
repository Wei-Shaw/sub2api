package service

import (
	"context"
	"time"
)

type ProxyLatencyInfo struct {
	Success               bool       `json:"success"`
	LatencyMs             *int64     `json:"latency_ms,omitempty"`
	Message               string     `json:"message,omitempty"`
	IPAddress             string     `json:"ip_address,omitempty"`
	Country               string     `json:"country,omitempty"`
	CountryCode           string     `json:"country_code,omitempty"`
	Region                string     `json:"region,omitempty"`
	City                  string     `json:"city,omitempty"`
	QualityStatus         string     `json:"quality_status,omitempty"`
	QualityScore          *int       `json:"quality_score,omitempty"`
	QualityGrade          string     `json:"quality_grade,omitempty"`
	QualitySummary        string     `json:"quality_summary,omitempty"`
	QualityCheckedAt      *int64     `json:"quality_checked_at,omitempty"`
	QualityCFRay          string     `json:"quality_cf_ray,omitempty"`
	GrokQualityStatus     string     `json:"grok_quality_status,omitempty"`
	GrokQualityCheckedAt  *time.Time `json:"grok_quality_checked_at,omitempty"`
	GrokQualityHTTPStatus *int       `json:"grok_quality_http_status,omitempty"`
	GrokQualityMessage    string     `json:"grok_quality_message,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ProxyLatencyCache interface {
	GetProxyLatencies(ctx context.Context, proxyIDs []int64) (map[int64]*ProxyLatencyInfo, error)
	SetProxyLatency(ctx context.Context, proxyID int64, info *ProxyLatencyInfo) error
}

func mergeCachedProxyQuality(dst, existing *ProxyLatencyInfo) {
	if dst == nil || existing == nil {
		return
	}
	if dst.QualityCheckedAt == nil &&
		dst.QualityScore == nil &&
		dst.QualityGrade == "" &&
		dst.QualityStatus == "" &&
		dst.QualitySummary == "" &&
		dst.QualityCFRay == "" {
		dst.QualityStatus = existing.QualityStatus
		dst.QualityScore = existing.QualityScore
		dst.QualityGrade = existing.QualityGrade
		dst.QualitySummary = existing.QualitySummary
		dst.QualityCheckedAt = existing.QualityCheckedAt
		dst.QualityCFRay = existing.QualityCFRay
	}
	if dst.GrokQualityCheckedAt == nil &&
		dst.GrokQualityStatus == "" &&
		dst.GrokQualityHTTPStatus == nil &&
		dst.GrokQualityMessage == "" {
		dst.GrokQualityStatus = existing.GrokQualityStatus
		dst.GrokQualityCheckedAt = existing.GrokQualityCheckedAt
		dst.GrokQualityHTTPStatus = existing.GrokQualityHTTPStatus
		dst.GrokQualityMessage = existing.GrokQualityMessage
	}
}
