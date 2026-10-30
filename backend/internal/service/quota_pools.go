package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// QuotaPool preserves upstream shared buckets: model aliases are never added together.
type QuotaPool struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Windows []QuotaPoolWindow `json:"windows"`
}
type QuotaPoolWindow struct {
	ID        string   `json:"id"`
	Remaining *float64 `json:"remaining"`
	ResetsAt  string   `json:"resets_at,omitempty"`
}

func quotaPoolsFromSummary(summary *antigravity.QuotaSummary) []QuotaPool {
	var pools []QuotaPool
	for _, group := range summary.Groups {
		pool := QuotaPool{Name: group.DisplayName}
		seen := map[string]bool{}
		for _, bucket := range group.Buckets {
			id, ok := strings.CutSuffix(bucket.ID, "-"+bucket.Window)
			if bucket.Window == "" || !ok || id == "" || (pool.ID != "" && pool.ID != id) || seen[bucket.Window] {
				continue
			}
			value := bucket.RemainingFraction
			if value != nil && (*value < 0 || *value > 1) {
				value = nil
			}
			pool.ID = id
			seen[bucket.Window] = true
			pool.Windows = append(pool.Windows, QuotaPoolWindow{ID: bucket.Window, Remaining: value, ResetsAt: bucket.ResetTime})
		}
		if len(pool.Windows) > 0 {
			if pool.ID == "gemini" {
				pool.Name = "Gemini"
			}
			if pool.ID == "3p" {
				pool.Name = "Claude / GPT"
			}
			pools = append(pools, pool)
		}
	}
	return pools
}
