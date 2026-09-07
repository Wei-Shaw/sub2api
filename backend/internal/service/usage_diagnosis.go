package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// UsageDiagnosisDetail is the admin-only diagnosis payload for one usage or error row.
type UsageDiagnosisDetail struct {
	Source string `json:"source"` // usage | error

	ID              int64      `json:"id"`
	RequestID       string     `json:"request_id"`
	ClientIP        string     `json:"client_ip,omitempty"`
	Path            string     `json:"path,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	StatusCode      int        `json:"status_code,omitempty"`
	Method          string     `json:"method,omitempty"`
	Stream          bool       `json:"stream"`
	DurationMs      *int       `json:"duration_ms,omitempty"`
	FirstTokenMs    *int       `json:"first_token_ms,omitempty"`
	RequestedModel  string     `json:"requested_model,omitempty"`
	UpstreamModel   string     `json:"upstream_model,omitempty"`
	APIKeyName      string     `json:"api_key_name,omitempty"`
	GroupName       string     `json:"group_name,omitempty"`
	InputTokens     int        `json:"input_tokens,omitempty"`
	OutputTokens    int        `json:"output_tokens,omitempty"`
	CacheReadTokens int        `json:"cache_read_tokens,omitempty"`
	TotalCost       float64    `json:"total_cost,omitempty"`
	ActualCost      float64    `json:"actual_cost,omitempty"`
	UpstreamURL     string     `json:"upstream_url,omitempty"`
	UpstreamStatus  int        `json:"upstream_status,omitempty"`

	HasDetail       bool              `json:"has_detail"`
	ReqHeaders      map[string]string `json:"req_headers,omitempty"`
	ResHeaders      map[string]string `json:"res_headers,omitempty"`
	ReqBody         string            `json:"req_body,omitempty"`
	ResBody         string            `json:"res_body,omitempty"`
	UpstreamReqBody string            `json:"upstream_req_body,omitempty"`
	Dialog          json.RawMessage   `json:"dialog,omitempty"`
	ErrorChain      json.RawMessage   `json:"error_chain,omitempty"`
}

func (s *UsageService) DumpStore() *UsageRequestDumpStore {
	if s == nil {
		return nil
	}
	return s.dumpStore
}

func (s *UsageService) ensureDumpStore() *UsageRequestDumpStore {
	if s == nil {
		return nil
	}
	if s.dumpStore == nil {
		s.dumpStore = DefaultUsageRequestDumpStore()
	}
	return s.dumpStore
}

// SaveRequestDump persists a diagnosis dump (auth headers masked). Best-effort.
func (s *UsageService) SaveRequestDump(d *UsageRequestDump) error {
	store := s.ensureDumpStore()
	if store == nil || d == nil {
		return nil
	}
	return store.Put(d)
}

// HasRequestDump reports whether a dump exists for request_id.
func (s *UsageService) HasRequestDump(requestID string) bool {
	store := s.ensureDumpStore()
	if store == nil {
		return false
	}
	return store.Has(requestID)
}

// HasRequestDumps returns the set of request_ids that have dumps.
func (s *UsageService) HasRequestDumps(requestIDs []string) map[string]bool {
	store := s.ensureDumpStore()
	if store == nil {
		return map[string]bool{}
	}
	return store.HasMany(requestIDs)
}

// GetUsageDiagnosis merges a usage log with its dump for admin diagnosis UI.
func (s *UsageService) GetUsageDiagnosis(ctx context.Context, id int64) (*UsageDiagnosisDetail, error) {
	log, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := usageLogToDiagnosis(log)
	s.mergeDump(detail, log.RequestID)
	return detail, nil
}

func usageLogToDiagnosis(log *UsageLog) *UsageDiagnosisDetail {
	if log == nil {
		return nil
	}
	d := &UsageDiagnosisDetail{
		Source:          "usage",
		ID:              log.ID,
		RequestID:       log.RequestID,
		CreatedAt:       log.CreatedAt,
		Stream:          log.Stream,
		DurationMs:      log.DurationMs,
		FirstTokenMs:    log.FirstTokenMs,
		RequestedModel:  firstNonEmptyStr(log.RequestedModel, log.Model),
		UpstreamModel:   optionalStringValue(log.UpstreamModel),
		InputTokens:     log.InputTokens,
		OutputTokens:    log.OutputTokens,
		CacheReadTokens: log.CacheReadTokens,
		TotalCost:       log.TotalCost,
		ActualCost:      log.ActualCost,
		Path:            optionalStringValue(log.InboundEndpoint),
		Method:          "POST",
	}
	if log.IPAddress != nil {
		d.ClientIP = strings.TrimSpace(*log.IPAddress)
	}
	if log.APIKey != nil {
		d.APIKeyName = log.APIKey.Name
	}
	if log.Group != nil {
		d.GroupName = log.Group.Name
	}
	if ep := optionalStringValue(log.UpstreamEndpoint); ep != "" {
		d.UpstreamURL = ep
	}
	return d
}

func (s *UsageService) mergeDump(detail *UsageDiagnosisDetail, requestID string) {
	if detail == nil {
		return
	}
	store := s.ensureDumpStore()
	if store == nil {
		return
	}
	dump, err := store.Get(requestID)
	if err != nil || dump == nil {
		return
	}
	detail.HasDetail = true
	if dump.Method != "" {
		detail.Method = dump.Method
	}
	if dump.Path != "" {
		detail.Path = dump.Path
	}
	if dump.StatusCode > 0 {
		detail.StatusCode = dump.StatusCode
	}
	if dump.UpstreamURL != "" {
		detail.UpstreamURL = dump.UpstreamURL
	}
	if dump.UpstreamStatus > 0 {
		detail.UpstreamStatus = dump.UpstreamStatus
	}
	detail.ReqHeaders = dump.ReqHeaders
	detail.ResHeaders = dump.ResHeaders
	detail.ReqBody = dump.ReqBody
	detail.ResBody = dump.ResBody
	detail.UpstreamReqBody = dump.UpstreamReqBody
	detail.Dialog = dump.Dialog
	detail.ErrorChain = dump.ErrorChain
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
