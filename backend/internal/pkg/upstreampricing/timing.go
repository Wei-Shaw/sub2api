package upstreampricing

// Cached station evidence, kept separate from request forwarding and billing.
import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

type TimingRecord struct {
	ID           string    `json:"id"`
	RequestID    string    `json:"request_id,omitempty"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"created_at"`
	DurationMs   *int64    `json:"duration_ms"`
	FirstTokenMs *int64    `json:"first_token_ms"`
	PrecisionMs  int64     `json:"precision_ms"`
}

type TimingComparison struct {
	Status       string         `json:"status"`
	Local        TimingRecord   `json:"local"`
	Station      *TimingRecord  `json:"station,omitempty"`
	Candidates   []TimingRecord `json:"candidates,omitempty"`
	DurationDiff *int64         `json:"duration_difference_ms"`
	FirstDiff    *int64         `json:"first_token_difference_ms"`
	CheckedAt    time.Time      `json:"checked_at"`
}

func timingInteger(v any) *int64 {
	n, err := strconv.ParseInt(stringValue(v), 10, 64)
	if err != nil || n < 0 || n > 365*24*60*60*1000 {
		return nil
	}
	return &n
}

func normalizeTiming(raw map[string]any, provider string) (TimingRecord, bool) {
	r := TimingRecord{ID: stringValue(raw["id"]), RequestID: stringValue(raw["request_id"]), Model: stringValue(raw["model"]), PrecisionMs: 1}
	if len(r.RequestID) > 300 || len(r.ID) > 120 {
		return r, false
	}
	if provider == "newapi" {
		r.Model = stringValue(raw["model_name"])
		stamp := timingInteger(raw["created_at"])
		if stamp == nil {
			return r, false
		}
		r.CreatedAt = time.Unix(*stamp, 0)
		r.DurationMs = timingInteger(raw["use_time"])
		if r.DurationMs != nil {
			if *r.DurationMs > 86400*7 {
				return r, false
			}
			*r.DurationMs *= 1000 // New API use_time is in seconds, frt in milliseconds.
		}
		other := object(raw["other"])
		if text, ok := raw["other"].(string); ok && len(text) <= 65536 {
			d := json.NewDecoder(strings.NewReader(text))
			d.UseNumber()
			_ = d.Decode(&other)
		}
		r.FirstTokenMs = timingInteger(other["frt"])
		r.PrecisionMs = 1000
	} else {
		var err error
		r.CreatedAt, err = time.Parse(time.RFC3339Nano, stringValue(raw["created_at"]))
		if err != nil {
			return r, false
		}
		r.DurationMs = timingInteger(raw["duration_ms"])
		r.FirstTokenMs = timingInteger(raw["first_token_ms"])
	}
	return r, r.ID != "" && validName(r.Model)
}

// Exact IDs are evidence. Similar timestamps/models are only candidates;
// they never receive a computed difference or replace the local measurement.
func compareTiming(local TimingRecord, upstreamID string, rows []TimingRecord) TimingComparison {
	out := TimingComparison{Status: "unmatched", Local: local, CheckedAt: time.Now()}
	exact := []TimingRecord{}
	for _, row := range rows {
		if row.RequestID != "" && (row.RequestID == upstreamID || row.RequestID == local.RequestID) {
			exact = append(exact, row)
		} else if row.Model == local.Model && math.Abs(row.CreatedAt.Sub(local.CreatedAt).Seconds()) <= 180 {
			if len(out.Candidates) < 5 {
				out.Candidates = append(out.Candidates, row)
			}
		}
	}
	if len(exact) == 1 {
		out.Status, out.Station, out.Candidates = "exact", &exact[0], nil
		if local.DurationMs != nil && exact[0].DurationMs != nil {
			v := *local.DurationMs - *exact[0].DurationMs
			out.DurationDiff = &v
		}
		if local.FirstTokenMs != nil && exact[0].FirstTokenMs != nil {
			v := *local.FirstTokenMs - *exact[0].FirstTokenMs
			out.FirstDiff = &v
		}
	} else if len(exact) > 1 {
		out.Status = "ambiguous"
		out.Candidates = exact[:min(5, len(exact))]
	} else if len(out.Candidates) > 0 {
		out.Status = "candidate"
	}
	return out
}

func (r *Remote) timingRecords(ctx context.Context, c Config, start, end time.Time) ([]TimingRecord, error) {
	return r.activityRecords(ctx, c, start, end)
}

func (m *Manager) CompareTiming(ctx context.Context, accountID, usageID int64) (*TimingComparison, error) {
	var local TimingRecord
	var upstreamID string
	var duration, first sql.NullInt64
	var payload []byte
	err := m.Store.DB.QueryRowContext(ctx, `SELECT u.id::text,u.request_id,COALESCE(u.upstream_request_id,''),
 COALESCE(NULLIF(u.upstream_model,''),u.model),u.created_at,u.duration_ms,u.first_token_ms,p.payload
 FROM usage_logs u LEFT JOIN fork_upstream_pricing_usage p ON p.usage_id=u.id WHERE u.id=$1 AND u.account_id=$2`, usageID, accountID).
		Scan(&local.ID, &local.RequestID, &upstreamID, &local.Model, &local.CreatedAt, &duration, &first, &payload)
	if err != nil {
		return nil, safeError("未找到该账号的用量记录，或用量扩展尚不可用")
	}
	local.PrecisionMs = 1
	if duration.Valid && duration.Int64 >= 0 {
		local.DurationMs = &duration.Int64
	}
	if first.Valid && first.Int64 >= 0 {
		local.FirstTokenMs = &first.Int64
	}
	src, err := m.Store.Source(ctx, accountID)
	if err != nil || !src.HasCredential {
		return nil, safeError("请先在账号的上游定价中保存面板读取凭证与准确分组")
	}
	fp, err := m.Store.AccountFingerprint(ctx, accountID)
	if err != nil || fp != src.AccountFingerprint {
		return nil, ErrConflict
	}
	// Require the immutable request snapshot so changed account credentials cannot
	// silently associate older traffic with another station's similarly timed logs.
	var snapshot Usage
	if json.Unmarshal(payload, &snapshot) != nil || snapshot.AccountFingerprint != fp {
		return nil, safeError("该历史请求缺少当前上游账号的原始快照，无法验证站点归属")
	}
	remote, err := m.Store.cachedTiming(ctx, accountID, activityIdentity(src), local.RequestID, upstreamID)
	if err != nil {
		return nil, safeError("上游明细缓存暂不可用")
	}
	after, err := m.Store.Source(ctx, accountID)
	currentFP, fpErr := m.Store.AccountFingerprint(ctx, accountID)
	if err != nil || fpErr != nil || after.Revision != src.Revision || currentFP != fp {
		return nil, ErrConflict
	}
	out := compareTiming(local, upstreamID, remote)
	return &out, nil
}
